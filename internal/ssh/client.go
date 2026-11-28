package sshclient

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	"vpscoop/internal/store"
)

// Upgrader upgrades HTTP connections to WebSocket for SSH terminal streaming.
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Dial establishes an SSH client connection using the host's configured auth method.
func Dial(host store.HostWithSecrets) (*ssh.Client, error) {
	var authMethods []ssh.AuthMethod

	if host.AuthType == "key" && strings.TrimSpace(host.PrivateKey) != "" {
		var signer ssh.Signer
		var err error
		if host.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(host.PrivateKey), []byte(host.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(host.PrivateKey))
		}
		if err != nil {
			return nil, fmt.Errorf("invalid private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if host.Password != "" {
		authMethods = append(authMethods, ssh.Password(host.Password))
		authMethods = append(authMethods, ssh.KeyboardInteractive(
			func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = host.Password
				}
				return answers, nil
			},
		))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication credentials configured for host %q", host.Name)
	}

	cfg := &ssh.ClientConfig{
		User:            host.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Personal VPS panel convenience
		Timeout:         12 * time.Second,
	}

	addr := net.JoinHostPort(host.Host.Host, strconv.Itoa(host.Port))
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	return client, nil
}

// DialWithKey tests connecting to a host exclusively using the provided private key and passphrase.
func DialWithKey(host store.HostWithSecrets, privateKey, passphrase string) (*ssh.Client, error) {
	var signer ssh.Signer
	var err error
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(privateKey), []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey([]byte(privateKey))
	}
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	cfg := &ssh.ClientConfig{
		User:            host.Username,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	addr := net.JoinHostPort(host.Host.Host, strconv.Itoa(host.Port))
	return ssh.Dial("tcp", addr, cfg)
}

type wsControlMsg struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
	Data string `json:"data"`
}

// HandleTerminalWS upgrades the HTTP connection to a WebSocket and bridges it to an interactive SSH PTY.
func HandleTerminalWS(w http.ResponseWriter, r *http.Request, host store.HostWithSecrets, onConnect func()) {
	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var writeMu sync.Mutex
	sendText := func(msg string) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(msg))
	}
	sendBinary := func(data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}

	sendText(fmt.Sprintf("\x1b[38;5;244mConnecting to %s@%s:%d...\x1b[0m\r\n", host.Username, host.Host.Host, host.Port))

	client, err := Dial(host)
	if err != nil {
		sendText(fmt.Sprintf("\r\n\x1b[31mConnection failed: %v\x1b[0m\r\n", err))
		return
	}
	defer client.Close()

	if onConnect != nil {
		onConnect()
	}

	session, err := client.NewSession()
	if err != nil {
		sendText(fmt.Sprintf("\r\n\x1b[31mFailed to create SSH session: %v\x1b[0m\r\n", err))
		return
	}
	defer session.Close()

	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	if cols <= 0 {
		cols = 100
	}
	if rows <= 0 {
		rows = 30
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sendText(fmt.Sprintf("\r\n\x1b[31mFailed to request PTY: %v\x1b[0m\r\n", err))
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		sendText(fmt.Sprintf("\r\n\x1b[31mFailed to open stdin: %v\x1b[0m\r\n", err))
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		sendText(fmt.Sprintf("\r\n\x1b[31mFailed to open stdout: %v\x1b[0m\r\n", err))
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		sendText(fmt.Sprintf("\r\n\x1b[31mFailed to open stderr: %v\x1b[0m\r\n", err))
		return
	}

	if err := session.Shell(); err != nil {
		sendText(fmt.Sprintf("\r\n\x1b[31mFailed to start shell: %v\x1b[0m\r\n", err))
		return
	}

	done := make(chan struct{})
	var once sync.Once
	closeDone := func() {
		once.Do(func() { close(done) })
	}

	pump := func(reader io.Reader) {
		buf := make([]byte, 8192)
		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				if err := sendBinary(buf[:n]); err != nil {
					closeDone()
					return
				}
			}
			if readErr != nil {
				closeDone()
				return
			}
		}
	}

	go pump(stdout)
	go pump(stderr)

	go func() {
		_ = session.Wait()
		closeDone()
	}()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
					closeDone()
					return
				}
			}
		}
	}()

	go func() {
		defer closeDone()
		for {
			_ = conn.SetReadDeadline(time.Now().Add(75 * time.Second))
			msgType, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.TextMessage {
				var ctrl wsControlMsg
				if len(payload) > 0 && payload[0] == '{' && json.Unmarshal(payload, &ctrl) == nil && ctrl.Type != "" {
					switch ctrl.Type {
					case "resize":
						if ctrl.Cols > 0 && ctrl.Rows > 0 {
							_ = session.WindowChange(ctrl.Rows, ctrl.Cols)
						}
					case "input":
						if ctrl.Data != "" {
							_, _ = stdin.Write([]byte(ctrl.Data))
						}
					case "ping":
						// no-op keepalive
					}
					continue
				}
			}
			if _, err := stdin.Write(payload); err != nil {
				return
			}
		}
	}()

	<-done
	sendText("\r\n\x1b[38;5;244m[SSH session closed]\x1b[0m\r\n")
}

// ProbeSpecs runs a lightweight read-only command over an open SSH client to detect CPU, RAM, Disk, and OS.
func ProbeSpecs(client *ssh.Client) string {
	sess, err := client.NewSession()
	if err != nil {
		return ""
	}
	defer sess.Close()

	var out strings.Builder
	sess.Stdout = &out

	const cmd = `CPU=$(nproc 2>/dev/null || grep -c ^processor /proc/cpuinfo 2>/dev/null || true); ` +
		`MEM=$(free -m 2>/dev/null | awk '/^Mem:/{print $2}'); ` +
		`DISK=$(df -k / 2>/dev/null | awk 'NR==2{printf "%d", $2/1048576}'); ` +
		`OS=$(grep '^PRETTY_NAME=' /etc/os-release 2>/dev/null | head -n 1 | cut -d= -f2- | tr -d '"'); ` +
		`ARCH=$(uname -m 2>/dev/null); ` +
		`echo "CPU=$CPU|MEM=$MEM|DISK=$DISK|OS=$OS|ARCH=$ARCH"`

	done := make(chan error, 1)
	go func() {
		done <- sess.Run(cmd)
	}()

	select {
	case err := <-done:
		if err != nil {
			return ""
		}
	case <-time.After(5 * time.Second):
		_ = sess.Close()
		return ""
	}

	kv := map[string]string{}
	for _, part := range strings.Split(strings.TrimSpace(out.String()), "|") {
		if k, v, ok := strings.Cut(part, "="); ok {
			kv[k] = strings.TrimSpace(v)
		}
	}

	var parts []string
	if cpu, err := strconv.Atoi(kv["CPU"]); err == nil && cpu > 0 {
		parts = append(parts, fmt.Sprintf("%d vCPU", cpu))
	}
	if memMB, err := strconv.Atoi(kv["MEM"]); err == nil && memMB > 0 {
		if memMB >= 1024 {
			gb := float64(memMB) / 1024.0
			if gb >= 10 {
				parts = append(parts, fmt.Sprintf("%.0fGB RAM", gb))
			} else {
				parts = append(parts, fmt.Sprintf("%.1fGB RAM", gb))
			}
		} else {
			parts = append(parts, fmt.Sprintf("%dMB RAM", memMB))
		}
	}
	if diskGB, err := strconv.Atoi(kv["DISK"]); err == nil && diskGB > 0 {
		parts = append(parts, fmt.Sprintf("%dGB Disk", diskGB))
	}
	if osName := kv["OS"]; osName != "" {
		if arch := kv["ARCH"]; arch != "" {
			parts = append(parts, fmt.Sprintf("%s (%s)", osName, arch))
		} else {
			parts = append(parts, osName)
		}
	} else if arch := kv["ARCH"]; arch != "" {
		parts = append(parts, arch)
	}
	return strings.Join(parts, " · ")
}
