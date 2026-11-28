package sshclient

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	"vpscoop/internal/store"
)

const ringBufferSize = 32 * 1024

type ringBuffer struct {
	buf []byte
	max int
}

func newRingBuffer(max int) *ringBuffer {
	return &ringBuffer{max: max}
}

func (r *ringBuffer) Write(p []byte) {
	if len(p) >= r.max {
		r.buf = append([]byte(nil), p[len(p)-r.max:]...)
		return
	}
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.max {
		r.buf = append([]byte(nil), r.buf[len(r.buf)-r.max:]...)
	}
}

func (r *ringBuffer) Bytes() []byte {
	out := make([]byte, len(r.buf))
	copy(out, r.buf)
	return out
}

// ManagedSession represents an active SSH PTY session kept alive in backend memory.
type ManagedSession struct {
	ID       string
	HostID   string
	HostAddr string
	client   *ssh.Client
	session  *ssh.Session
	stdin    io.WriteCloser

	mu         sync.Mutex
	activeConn *websocket.Conn
	ring       *ringBuffer
	closed     bool
	done       chan struct{}
	once       sync.Once
	onClose    func(id string)
}

func (m *ManagedSession) closeDone() {
	m.once.Do(func() {
		close(m.done)
	})
}

// Attach binds a WebSocket connection to this live SSH session and replays recent terminal output.
func (m *ManagedSession) Attach(conn *websocket.Conn, cols, rows int) {
	m.mu.Lock()
	old := m.activeConn
	m.activeConn = conn
	history := m.ring.Bytes()
	m.mu.Unlock()

	if old != nil && old != conn {
		_ = old.Close()
	}

	if len(history) > 0 {
		m.mu.Lock()
		if m.activeConn == conn {
			_ = conn.WriteMessage(websocket.BinaryMessage, history)
		}
		m.mu.Unlock()
	}

	if cols > 0 && rows > 0 {
		_ = m.session.WindowChange(rows, cols)
	}
}

// Detach unbinds the WebSocket without closing the underlying SSH session.
func (m *ManagedSession) Detach(conn *websocket.Conn) {
	m.mu.Lock()
	if m.activeConn == conn {
		m.activeConn = nil
	}
	m.mu.Unlock()
}

func (m *ManagedSession) sendBinary(data []byte) {
	m.mu.Lock()
	m.ring.Write(data)
	conn := m.activeConn
	if conn != nil {
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			m.activeConn = nil
		}
	}
	m.mu.Unlock()
}

func (m *ManagedSession) sendText(msg string) {
	m.mu.Lock()
	conn := m.activeConn
	if conn != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(msg))
	}
	m.mu.Unlock()
}

// Close terminates the SSH session and cleans up resources.
func (m *ManagedSession) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	conn := m.activeConn
	m.activeConn = nil
	m.mu.Unlock()

	m.closeDone()
	if conn != nil {
		_ = conn.Close()
	}
	_ = m.stdin.Close()
	_ = m.session.Close()
	_ = m.client.Close()
	if m.onClose != nil {
		m.onClose(m.ID)
	}
}

// Manager holds persistent SSH sessions in memory so browser refreshes do not disconnect them.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*ManagedSession
}

// NewManager creates an in-memory SSH session Manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*ManagedSession),
	}
}

// GetOrCreate returns an existing live session for sessionID, or dials and starts a new one.
func (mgr *Manager) GetOrCreate(sessionID string, host store.HostWithSecrets, cols, rows int, onConnect func()) (*ManagedSession, bool, error) {
	if sessionID != "" {
		mgr.mu.RLock()
		existing, ok := mgr.sessions[sessionID]
		mgr.mu.RUnlock()
		if ok {
			select {
			case <-existing.done:
				mgr.CloseSession(sessionID)
			default:
				return existing, true, nil
			}
		}
	}

	client, err := Dial(host)
	if err != nil {
		return nil, false, err
	}
	if onConnect != nil {
		onConnect()
	}

	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, false, err
	}

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
		_ = session.Close()
		_ = client.Close()
		return nil, false, fmt.Errorf("request PTY: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, false, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, false, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, false, err
	}

	if err := session.Shell(); err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, false, fmt.Errorf("start shell: %w", err)
	}

	ms := &ManagedSession{
		ID:       sessionID,
		HostID:   host.ID,
		HostAddr: fmt.Sprintf("%s@%s:%d", host.Username, host.Host.Host, host.Port),
		client:   client,
		session:  session,
		stdin:    stdin,
		ring:     newRingBuffer(ringBufferSize),
		done:     make(chan struct{}),
		onClose: func(id string) {
			if id == "" {
				return
			}
			mgr.mu.Lock()
			if cur, exists := mgr.sessions[id]; exists && cur.closed {
				delete(mgr.sessions, id)
			}
			mgr.mu.Unlock()
		},
	}

	if sessionID != "" {
		mgr.mu.Lock()
		mgr.sessions[sessionID] = ms
		mgr.mu.Unlock()
	}

	pump := func(r io.Reader) {
		buf := make([]byte, 8192)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				ms.sendBinary(buf[:n])
			}
			if readErr != nil {
				ms.closeDone()
				return
			}
		}
	}

	go pump(stdout)
	go pump(stderr)

	go func() {
		_ = session.Wait()
		ms.sendText("\r\n\x1b[38;5;244m[SSH session closed]\x1b[0m\r\n")
		ms.Close()
	}()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ms.done:
				return
			case <-ticker.C:
				if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
					ms.Close()
					return
				}
			}
		}
	}()

	return ms, false, nil
}

// CloseSession terminates a managed SSH session by ID.
func (mgr *Manager) CloseSession(sessionID string) {
	mgr.mu.RLock()
	ms, ok := mgr.sessions[sessionID]
	mgr.mu.RUnlock()
	if ok {
		ms.Close()
	}
}

// CloseAll terminates all active SSH sessions (e.g., when locking the panel).
func (mgr *Manager) CloseAll() {
	mgr.mu.RLock()
	list := make([]*ManagedSession, 0, len(mgr.sessions))
	for _, ms := range mgr.sessions {
		list = append(list, ms)
	}
	mgr.mu.RUnlock()

	for _, ms := range list {
		ms.Close()
	}
}

// ServeWS attaches a WebSocket client to a persistent ManagedSession.
// Closing the WebSocket detaches without killing the SSH session unless sessionID is empty.
func (mgr *Manager) ServeWS(conn *websocket.Conn, sessionID string, host store.HostWithSecrets, cols, rows int, onConnect func()) {
	defer conn.Close()

	if sessionID == "" {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(
			fmt.Sprintf("\x1b[38;5;244mConnecting to %s@%s:%d...\x1b[0m\r\n", host.Username, host.Host.Host, host.Port),
		))
	}

	ms, reattached, err := mgr.GetOrCreate(sessionID, host, cols, rows, onConnect)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(
			fmt.Sprintf("\r\n\x1b[31mConnection failed: %v\x1b[0m\r\n", err),
		))
		return
	}

	if reattached {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(
			"\x1b[38;5;244m[Reattached to active SSH session]\x1b[0m\r\n",
		))
	}

	ms.Attach(conn, cols, rows)
	defer func() {
		ms.Detach(conn)
		if sessionID == "" {
			ms.Close()
		}
	}()

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
						_ = ms.session.WindowChange(ctrl.Rows, ctrl.Cols)
					}
				case "input":
					if ctrl.Data != "" {
						_, _ = ms.stdin.Write([]byte(ctrl.Data))
					}
				case "close":
					ms.Close()
					return
				case "ping":
					// keepalive
				}
				continue
			}
		}
		if _, err := ms.stdin.Write(payload); err != nil {
			return
		}
	}
}
