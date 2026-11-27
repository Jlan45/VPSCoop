package tailscale

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	sshclient "vpscoop/internal/ssh"
	"vpscoop/internal/store"
)

// StatusResult describes Tailscale state on a remote VPS.
type StatusResult struct {
	Installed   bool   `json:"installed"`
	Running     bool   `json:"running"`
	TailscaleIP string `json:"tailscaleIp"`
	Hostname    string `json:"hostname"`
	Output      string `json:"output,omitempty"`
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var nonHostChar = regexp.MustCompile(`[^a-zA-Z0-9-]+`)

// SanitizeHostname converts a VPS name into a valid Tailscale machine hostname.
func SanitizeHostname(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonHostChar.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "vpscoop-node"
	}
	if len(s) > 48 {
		s = strings.TrimRight(s[:48], "-")
	}
	return s
}

func sudoPrefix(username string) string {
	if strings.TrimSpace(username) == "root" {
		return ""
	}
	return "sudo "
}

// CheckStatus checks whether Tailscale is installed and connected on the host.
func CheckStatus(host store.HostWithSecrets) (StatusResult, error) {
	client, err := sshclient.Dial(host)
	if err != nil {
		return StatusResult{}, fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return StatusResult{}, err
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	sudo := sudoPrefix(host.Username)
	cmd := fmt.Sprintf(
		"if command -v tailscale >/dev/null 2>&1; then "+
			"echo INSTALLED=1; "+
			"IP=$(%stailscale ip -4 2>/dev/null | head -n 1); "+
			"echo IP=$IP; "+
			"echo HOST=$(hostname 2>/dev/null); "+
			"else echo INSTALLED=0; fi",
		sudo,
	)
	if err := sess.Run(cmd); err != nil {
		return StatusResult{}, fmt.Errorf("check tailscale: %s (%v)", strings.TrimSpace(stderr.String()), err)
	}

	res := StatusResult{Output: strings.TrimSpace(stdout.String())}
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "INSTALLED=1") {
			res.Installed = true
		} else if strings.HasPrefix(line, "IP=") {
			res.TailscaleIP = strings.TrimSpace(strings.TrimPrefix(line, "IP="))
			if res.TailscaleIP != "" {
				res.Running = true
			}
		} else if strings.HasPrefix(line, "HOST=") {
			res.Hostname = strings.TrimSpace(strings.TrimPrefix(line, "HOST="))
		}
	}
	return res, nil
}

// InstallAndUp installs Tailscale if missing, runs `tailscale up` with authKey, and returns the assigned 100.x IP.
func InstallAndUp(host store.HostWithSecrets, authKey, hostname string, enableSSH bool) (StatusResult, error) {
	if strings.TrimSpace(authKey) == "" {
		return StatusResult{}, fmt.Errorf("tailscale authKey is required")
	}

	client, err := sshclient.Dial(host)
	if err != nil {
		return StatusResult{}, fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return StatusResult{}, err
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	sudo := sudoPrefix(host.Username)
	safeHost := SanitizeHostname(hostname)
	if safeHost == "" {
		safeHost = SanitizeHostname(host.Name)
	}

	sshFlag := ""
	if enableSSH {
		sshFlag = " --ssh"
	}

	script := fmt.Sprintf(
		"set -e\n"+
			"if ! command -v tailscale >/dev/null 2>&1; then\n"+
			"  curl -fsSL https://tailscale.com/install.sh | %ssh\n"+
			"fi\n"+
			"%stailscale up --authkey=%s --hostname=%s%s --reset\n"+
			"IP=$(%stailscale ip -4 | head -n 1)\n"+
			"echo \"IP=$IP\"\n",
		sudo,
		sudo, shellQuote(strings.TrimSpace(authKey)), shellQuote(safeHost), sshFlag,
		sudo,
	)

	if err := sess.Run(script); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return StatusResult{Output: stdout.String()}, fmt.Errorf("tailscale setup failed: %s", msg)
	}

	res := StatusResult{
		Installed: true,
		Hostname:  safeHost,
		Output:    strings.TrimSpace(stdout.String()),
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "IP=") {
			res.TailscaleIP = strings.TrimSpace(strings.TrimPrefix(line, "IP="))
			if res.TailscaleIP != "" {
				res.Running = true
			}
		}
	}
	return res, nil
}
