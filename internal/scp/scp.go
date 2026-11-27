package scp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

// FileEntry describes a file or directory on the remote host.
type FileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDir     bool   `json:"isDir"`
	IsSymlink bool   `json:"isSymlink"`
	Size      int64  `json:"size"`
	Mode      string `json:"mode"`
	ModTime   string `json:"modTime"`
	Owner     string `json:"owner"`
	Group     string `json:"group"`
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func runCommand(client *ssh.Client, cmd string) (string, string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	err = session.Run(cmd)
	return stdout.String(), stderr.String(), err
}

// ListDir lists the directory contents at remotePath and returns the resolved absolute path.
func ListDir(client *ssh.Client, remotePath string) ([]FileEntry, string, error) {
	target := strings.TrimSpace(remotePath)
	if target == "" || target == "~" {
		target = "$HOME"
	} else if strings.HasPrefix(target, "~/") {
		target = "$HOME/" + shellQuote(strings.TrimPrefix(target, "~/"))
	} else {
		target = shellQuote(target)
	}

	// Try GNU ls with --time-style=long-iso first, fallback to portable ls -la
	cmd := fmt.Sprintf("cd %s && pwd && (LC_ALL=C ls -la --time-style=long-iso 2>/dev/null || LC_ALL=C ls -la)", target)
	out, stderr, err := runCommand(client, cmd)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return nil, "", fmt.Errorf("list directory: %s", msg)
	}

	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return []FileEntry{}, "/", nil
	}

	resolvedDir := strings.TrimSpace(lines[0])
	if resolvedDir == "" {
		resolvedDir = "/"
	}

	entries := []FileEntry{}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}
		entry, ok := parseLsLine(line, resolvedDir)
		if !ok || entry.Name == "." || entry.Name == ".." {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, resolvedDir, nil
}

func parseLsLine(line string, dir string) (FileEntry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 8 {
		return FileEntry{}, false
	}

	mode := fields[0]
	isDir := strings.HasPrefix(mode, "d")
	isSymlink := strings.HasPrefix(mode, "l")
	owner := fields[2]
	group := fields[3]
	size, _ := strconv.ParseInt(fields[4], 10, 64)

	var modTime string
	var nameStartIdx int

	// Check if --time-style=long-iso format: YYYY-MM-DD HH:MM
	if len(fields) >= 8 && len(fields[5]) == 10 && fields[5][4] == '-' && fields[5][7] == '-' {
		modTime = fields[5] + " " + fields[6]
		nameStartIdx = 7
	} else if len(fields) >= 9 {
		// Standard ls -la format: Month Day TimeOrYear
		modTime = fields[5] + " " + fields[6] + " " + fields[7]
		nameStartIdx = 8
	} else {
		return FileEntry{}, false
	}

	// Extract name preserving spaces by finding the nth field offset
	name := extractTailAfterFields(line, nameStartIdx)
	if isSymlink {
		if idx := strings.Index(name, " -> "); idx > 0 {
			name = name[:idx]
		}
	}
	if name == "" {
		return FileEntry{}, false
	}

	return FileEntry{
		Name:      name,
		Path:      path.Join(dir, name),
		IsDir:     isDir,
		IsSymlink: isSymlink,
		Size:      size,
		Mode:      mode,
		ModTime:   modTime,
		Owner:     owner,
		Group:     group,
	}, true
}

func extractTailAfterFields(line string, fieldCount int) string {
	i := 0
	n := len(line)
	for f := 0; f < fieldCount; f++ {
		for i < n && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		for i < n && line[i] != ' ' && line[i] != '\t' {
			i++
		}
	}
	for i < n && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return line[i:]
}

// Download streams a remote file using the SCP protocol (scp -f) into w.
func Download(client *ssh.Client, remotePath string, w io.Writer) (string, int64, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", 0, err
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return "", 0, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", 0, err
	}
	var stderr bytes.Buffer
	session.Stderr = &stderr

	if err := session.Start("scp -f " + shellQuote(remotePath)); err != nil {
		return "", 0, err
	}

	ack := func() error {
		_, writeErr := stdin.Write([]byte{0})
		return writeErr
	}

	if err := ack(); err != nil {
		return "", 0, err
	}

	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg != "" {
				return "", 0, errors.New(errMsg)
			}
			return "", 0, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		switch line[0] {
		case 1, 2:
			return "", 0, errors.New(line[1:])
		case 'T':
			if err := ack(); err != nil {
				return "", 0, err
			}
			continue
		case 'C':
			parts := strings.SplitN(line[1:], " ", 3)
			if len(parts) != 3 {
				return "", 0, fmt.Errorf("unexpected scp header: %q", line)
			}
			size, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return "", 0, fmt.Errorf("invalid scp file size: %w", err)
			}
			filename := parts[2]

			if err := ack(); err != nil {
				return "", 0, err
			}

			if _, err := io.CopyN(w, reader, size); err != nil {
				return "", 0, err
			}

			b, err := reader.ReadByte()
			if err != nil {
				return "", 0, err
			}
			if b != 0 {
				msg, _ := reader.ReadString('\n')
				return "", 0, fmt.Errorf("scp transfer error: %s", strings.TrimSpace(msg))
			}

			_ = ack()
			_ = stdin.Close()
			_ = session.Wait()
			return filename, size, nil
		default:
			return "", 0, fmt.Errorf("unexpected scp response: %q", line)
		}
	}
}

func readAck(r *bufio.Reader) error {
	b, err := r.ReadByte()
	if err != nil {
		return err
	}
	if b == 0 {
		return nil
	}
	msg, _ := r.ReadString('\n')
	if msg == "" {
		return fmt.Errorf("scp error code %d", b)
	}
	return errors.New(strings.TrimSpace(msg))
}

// Upload sends a file to remoteDir using the SCP protocol (scp -t).
func Upload(client *ssh.Client, remoteDir, filename string, size int64, r io.Reader) error {
	safeName := path.Base(strings.TrimSpace(filename))
	if safeName == "" || safeName == "." || safeName == ".." {
		return errors.New("invalid filename")
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	session.Stderr = &stderr

	if err := session.Start("scp -t " + shellQuote(remoteDir)); err != nil {
		return err
	}

	reader := bufio.NewReader(stdout)
	if err := readAck(reader); err != nil {
		return fmt.Errorf("scp init: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	header := fmt.Sprintf("C0644 %d %s\n", size, safeName)
	if _, err := io.WriteString(stdin, header); err != nil {
		return err
	}
	if err := readAck(reader); err != nil {
		return fmt.Errorf("scp header: %w", err)
	}

	if _, err := io.CopyN(stdin, r, size); err != nil {
		return err
	}
	if _, err := stdin.Write([]byte{0}); err != nil {
		return err
	}
	if err := readAck(reader); err != nil {
		return fmt.Errorf("scp finish: %w", err)
	}

	_ = stdin.Close()
	return session.Wait()
}

// Mkdir creates a directory on the remote host.
func Mkdir(client *ssh.Client, remotePath string) error {
	_, stderr, err := runCommand(client, "mkdir -p "+shellQuote(remotePath))
	if err != nil {
		return fmt.Errorf("mkdir: %s", strings.TrimSpace(stderr))
	}
	return nil
}

// Rename moves or renames a file or directory on the remote host.
func Rename(client *ssh.Client, oldPath, newPath string) error {
	_, stderr, err := runCommand(client, "mv "+shellQuote(oldPath)+" "+shellQuote(newPath))
	if err != nil {
		return fmt.Errorf("rename: %s", strings.TrimSpace(stderr))
	}
	return nil
}

// Delete removes a file or directory on the remote host.
func Delete(client *ssh.Client, remotePath string) error {
	clean := path.Clean(strings.TrimSpace(remotePath))
	if clean == "" || clean == "/" || clean == "." {
		return errors.New("refusing to delete root or empty path")
	}
	_, stderr, err := runCommand(client, "rm -rf "+shellQuote(clean))
	if err != nil {
		return fmt.Errorf("delete: %s", strings.TrimSpace(stderr))
	}
	return nil
}

// ReadFileText downloads a small file via SCP for inline preview/editing.
func ReadFileText(client *ssh.Client, remotePath string, maxBytes int64) (string, error) {
	var buf bytes.Buffer
	_, size, err := Download(client, remotePath, &limitedWriter{w: &buf, max: maxBytes})
	if err != nil {
		return "", err
	}
	if size > maxBytes {
		return "", fmt.Errorf("file too large to preview (%d bytes, max %d bytes)", size, maxBytes)
	}
	return buf.String(), nil
}

type limitedWriter struct {
	w   io.Writer
	max int64
	n   int64
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n+int64(len(p)) > l.max {
		return 0, fmt.Errorf("file exceeds preview limit of %d bytes", l.max)
	}
	n, err := l.w.Write(p)
	l.n += int64(n)
	return n, err
}
