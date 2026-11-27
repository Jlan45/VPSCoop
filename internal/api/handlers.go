package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"vpscoop/internal/backup"
	"vpscoop/internal/crypto"
	"vpscoop/internal/importer"
	"vpscoop/internal/keys"
	"vpscoop/internal/scp"
	sshclient "vpscoop/internal/ssh"
	"vpscoop/internal/store"
	"vpscoop/internal/tailscale"
	"vpscoop/internal/webdav"
)

// Server bundles HTTP handlers for the VPSCoop API.
type Server struct {
	mu            sync.RWMutex
	store         *store.Store
	dataDir       string
	sessionSecret []byte
}

// NewServer creates an API Server backed by s and storing uploaded files under dataDir.
func NewServer(s *store.Store, dataDir string) *Server {
	sec := make([]byte, 32)
	_, _ = rand.Read(sec)
	return &Server{store: s, dataDir: dataDir, sessionSecret: sec}
}

const sessionCookieName = "vpscoop_session"

func (s *Server) issueSessionCookie(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	sec := s.sessionSecret
	s.mu.RUnlock()

	exp := time.Now().Add(7 * 24 * time.Hour).Unix()
	expStr := strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, sec)
	mac.Write([]byte(expStr))
	sig := hex.EncodeToString(mac.Sum(nil))

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    expStr + "." + sig,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		MaxAge:   7 * 24 * 3600,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *Server) isAuthenticated(r *http.Request) bool {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	s.mu.RLock()
	sec := s.sessionSecret
	s.mu.RUnlock()

	mac := hmac.New(sha256.New, sec)
	mac.Write([]byte(parts[0]))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}

func (s *Server) requireInit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		st := s.store
		s.mu.RUnlock()
		if st == nil {
			writeError(w, http.StatusLocked, "面板已锁定，请先输入主密钥解锁")
			return
		}
		if !s.isAuthenticated(r) {
			writeError(w, http.StatusUnauthorized, "未登录或会话已过期，请先输入主密钥验证")
			return
		}
		next(w, r)
	}
}

// RegisterRoutes attaches all API routes to mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/setup", s.handleAuthSetup)
	mux.HandleFunc("POST /api/auth/unlock", s.handleAuthUnlock)
	mux.HandleFunc("POST /api/auth/lock", s.handleAuthLock)
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)

	mux.HandleFunc("GET /api/hosts", s.requireInit(s.handleListHosts))
	mux.HandleFunc("POST /api/hosts", s.requireInit(s.handleCreateHost))
	mux.HandleFunc("POST /api/hosts/import", s.requireInit(s.handleImportHosts))
	mux.HandleFunc("PUT /api/hosts/{id}", s.requireInit(s.handleUpdateHost))
	mux.HandleFunc("DELETE /api/hosts/{id}", s.requireInit(s.handleDeleteHost))
	mux.HandleFunc("POST /api/hosts/{id}/test", s.requireInit(s.handleTestHost))
	mux.HandleFunc("POST /api/hosts/{id}/key-check", s.requireInit(s.handleCheckHostKey))
	mux.HandleFunc("POST /api/hosts/{id}/key-inject", s.requireInit(s.handleInjectHostKey))

	mux.HandleFunc("GET /api/ws/ssh/{id}", s.requireInit(s.handleSSHTerminal))

	mux.HandleFunc("GET /api/scp/{id}/list", s.requireInit(s.handleSCPList))
	mux.HandleFunc("GET /api/scp/{id}/download", s.requireInit(s.handleSCPDownload))
	mux.HandleFunc("POST /api/scp/{id}/upload", s.requireInit(s.handleSCPUpload))
	mux.HandleFunc("POST /api/scp/{id}/mkdir", s.requireInit(s.handleSCPMkdir))
	mux.HandleFunc("POST /api/scp/{id}/rename", s.requireInit(s.handleSCPRename))
	mux.HandleFunc("POST /api/scp/{id}/delete", s.requireInit(s.handleSCPDelete))
	mux.HandleFunc("GET /api/scp/{id}/read", s.requireInit(s.handleSCPRead))
	mux.HandleFunc("POST /api/scp/{id}/write", s.requireInit(s.handleSCPWrite))

	mux.HandleFunc("GET /api/keys", s.requireInit(s.handleListKeys))
	mux.HandleFunc("POST /api/keys/generate", s.requireInit(s.handleGenerateKey))
	mux.HandleFunc("POST /api/keys", s.requireInit(s.handleImportKey))
	mux.HandleFunc("GET /api/keys/{id}/secret", s.requireInit(s.handleGetKeySecret))
	mux.HandleFunc("PUT /api/keys/{id}", s.requireInit(s.handleUpdateKey))
	mux.HandleFunc("GET /api/keys/{id}/private", s.requireInit(s.handleDownloadPrivateKey))
	mux.HandleFunc("DELETE /api/keys/{id}", s.requireInit(s.handleDeleteKey))

	mux.HandleFunc("GET /api/hosts/{id}/invoices", s.requireInit(s.handleListInvoices))
	mux.HandleFunc("POST /api/hosts/{id}/invoices", s.requireInit(s.handleUploadInvoice))
	mux.HandleFunc("GET /api/invoices/{id}/file", s.requireInit(s.handleGetInvoiceFile))
	mux.HandleFunc("DELETE /api/invoices/{id}", s.requireInit(s.handleDeleteInvoice))

	mux.HandleFunc("GET /api/settings/tailscale", s.requireInit(s.handleGetTailscaleSetting))
	mux.HandleFunc("PUT /api/settings/tailscale", s.requireInit(s.handleSaveTailscaleSetting))
	mux.HandleFunc("POST /api/hosts/{id}/tailscale/check", s.requireInit(s.handleCheckTailscale))
	mux.HandleFunc("POST /api/hosts/{id}/tailscale/inject", s.requireInit(s.handleInjectTailscale))

	mux.HandleFunc("POST /api/backup/export", s.requireInit(s.handleExportBackup))
	mux.HandleFunc("POST /api/backup/import", s.requireInit(s.handleImportBackup))

	mux.HandleFunc("GET /api/cloud/webdav", s.requireInit(s.handleGetWebDAV))
	mux.HandleFunc("PUT /api/cloud/webdav", s.requireInit(s.handleSaveWebDAV))
	mux.HandleFunc("POST /api/cloud/webdav/test", s.requireInit(s.handleTestWebDAV))
	mux.HandleFunc("GET /api/cloud/webdav/files", s.requireInit(s.handleListWebDAV))
	mux.HandleFunc("POST /api/cloud/webdav/push", s.requireInit(s.handlePushWebDAV))
	mux.HandleFunc("POST /api/cloud/webdav/pull", s.requireInit(s.handlePullWebDAV))
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.store.ListHosts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hosts": hosts})
}

func (s *Server) handleCreateHost(w http.ResponseWriter, r *http.Request) {
	var in store.HostInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Host) == "" {
		writeError(w, http.StatusBadRequest, "name and host are required")
		return
	}
	created, err := s.store.CreateHost(in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in store.HostInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Host) == "" {
		writeError(w, http.StatusBadRequest, "name and host are required")
		return
	}
	updated, err := s.store.UpdateHost(id, in)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.store.DeleteHost(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleTestHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	endpointID := r.URL.Query().Get("endpointId")
	host, err := s.store.GetHostWithEndpoint(id, endpointID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	start := time.Now()
	client, err := sshclient.Dial(host)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer client.Close()
	latency := time.Since(start).Milliseconds()
	s.store.TouchLastConnected(id)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"latencyMs": latency,
	})
}

func (s *Server) handleSSHTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	endpointID := r.URL.Query().Get("endpointId")
	host, err := s.store.GetHostWithEndpoint(id, endpointID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sshclient.HandleTerminalWS(w, r, host, func() {
		s.store.TouchLastConnected(id)
	})
}

func (s *Server) dialForSCP(w http.ResponseWriter, r *http.Request, id string) (store.HostWithSecrets, func(), bool) {
	endpointID := r.URL.Query().Get("endpointId")
	host, err := s.store.GetHostWithEndpoint(id, endpointID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "host not found")
		return host, nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return host, nil, false
	}
	return host, func() {}, true
}

func (s *Server) handleSCPList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	host, _, ok := s.dialForSCP(w, r, id)
	if !ok {
		return
	}
	client, err := sshclient.Dial(host)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer client.Close()

	remotePath := r.URL.Query().Get("path")
	entries, resolvedPath, err := scp.ListDir(client, remotePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    resolvedPath,
		"entries": entries,
	})
}

func (s *Server) handleSCPDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	remotePath := r.URL.Query().Get("path")
	if strings.TrimSpace(remotePath) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	host, _, ok := s.dialForSCP(w, r, id)
	if !ok {
		return
	}
	client, err := sshclient.Dial(host)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer client.Close()

	filename := path.Base(remotePath)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(filename, `"`, "")))
	w.Header().Set("Content-Type", "application/octet-stream")

	if _, _, err := scp.Download(client, remotePath, w); err != nil {
		// Headers may already be sent if streaming started; best effort error
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}

func (s *Server) handleSCPUpload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	remoteDir := strings.TrimSpace(r.FormValue("dir"))
	if remoteDir == "" {
		remoteDir = "~"
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	host, _, ok := s.dialForSCP(w, r, id)
	if !ok {
		return
	}
	client, err := sshclient.Dial(host)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer client.Close()

	if err := scp.Upload(client, remoteDir, header.Filename, header.Size, file); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSCPMkdir(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Path) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	host, _, ok := s.dialForSCP(w, r, id)
	if !ok {
		return
	}
	client, err := sshclient.Dial(host)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer client.Close()

	if err := scp.Mkdir(client, body.Path); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSCPRename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.OldPath == "" || body.NewPath == "" {
		writeError(w, http.StatusBadRequest, "oldPath and newPath are required")
		return
	}
	host, _, ok := s.dialForSCP(w, r, id)
	if !ok {
		return
	}
	client, err := sshclient.Dial(host)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer client.Close()

	if err := scp.Rename(client, body.OldPath, body.NewPath); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSCPDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Path) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	host, _, ok := s.dialForSCP(w, r, id)
	if !ok {
		return
	}
	client, err := sshclient.Dial(host)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer client.Close()

	if err := scp.Delete(client, body.Path); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSCPRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	remotePath := r.URL.Query().Get("path")
	if strings.TrimSpace(remotePath) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	host, _, ok := s.dialForSCP(w, r, id)
	if !ok {
		return
	}
	client, err := sshclient.Dial(host)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer client.Close()

	content, err := scp.ReadFileText(client, remotePath, 512*1024)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

func (s *Server) handleSCPWrite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Dir      string `json:"dir"`
		Filename string `json:"filename"`
		Content  string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Dir == "" || body.Filename == "" {
		writeError(w, http.StatusBadRequest, "dir and filename are required")
		return
	}
	host, _, ok := s.dialForSCP(w, r, id)
	if !ok {
		return
	}
	client, err := sshclient.Dial(host)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer client.Close()

	data := []byte(body.Content)
	if err := scp.Upload(client, body.Dir, body.Filename, int64(len(data)), bytes.NewReader(data)); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}


func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListKeys()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": list})
}

func (s *Server) handleGenerateKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		Algorithm  string `json:"algorithm"`
		Comment    string `json:"comment"`
		Passphrase string `json:"passphrase"`
		DryRun     bool   `json:"dryRun"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	gen, err := keys.Generate(body.Algorithm, body.Comment, body.Passphrase)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if body.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{
			"algorithm":   gen.Algorithm,
			"publicKey":   gen.PublicKey,
			"privateKey":  gen.PrivateKey,
			"fingerprint": gen.Fingerprint,
		})
		return
	}

	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	created, err := s.store.CreateKey(
		body.Name, gen.Algorithm, gen.PublicKey, gen.PrivateKey,
		body.Passphrase, gen.Fingerprint, body.Comment,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetKeySecret(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	k, err := s.store.GetKeyWithSecret(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (s *Server) handleUpdateKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Name       string `json:"name"`
		Algorithm  string `json:"algorithm"`
		PublicKey  string `json:"publicKey"`
		PrivateKey string `json:"privateKey"`
		Passphrase string `json:"passphrase"`
		Comment    string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	var alg, pub, fp string
	if strings.TrimSpace(body.PrivateKey) != "" {
		parsed, err := keys.ParseExisting(body.PrivateKey, body.Passphrase, body.Comment)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		alg = parsed.Algorithm
		pub = parsed.PublicKey
		fp = parsed.Fingerprint
	} else {
		alg = body.Algorithm
		pub = body.PublicKey
	}

	updated, err := s.store.UpdateKey(id, body.Name, alg, pub, body.PrivateKey, body.Passphrase, fp, body.Comment)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleImportKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		PrivateKey string `json:"privateKey"`
		Passphrase string `json:"passphrase"`
		Comment    string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	parsed, err := keys.ParseExisting(body.PrivateKey, body.Passphrase, body.Comment)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := s.store.CreateKey(
		body.Name, parsed.Algorithm, parsed.PublicKey, parsed.PrivateKey,
		body.Passphrase, parsed.Fingerprint, body.Comment,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleDownloadPrivateKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	k, err := s.store.GetKeyWithSecret(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	safeName := strings.NewReplacer(" ", "_", "/", "_", `"`, "").Replace(k.Name)
	if safeName == "" {
		safeName = "id_key"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pem"`, safeName))
	w.Header().Set("Content-Type", "application/x-pem-file")
	_, _ = w.Write([]byte(k.PrivateKey))
}

func (s *Server) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.store.DeleteKey(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}


func (s *Server) handleCheckHostKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		KeyID string `json:"keyId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	host, err := s.store.GetHostWithSecrets(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	keyID := body.KeyID
	if keyID == "" {
		keyID = host.KeyID
	}
	if keyID == "" {
		writeError(w, http.StatusBadRequest, "keyId is required")
		return
	}

	key, err := s.store.GetKeyWithSecret(keyID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res := keys.CheckOnHost(host, key)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleInjectHostKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		KeyID          string `json:"keyId"`
		SwitchHostAuth *bool  `json:"switchHostAuth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.KeyID == "" {
		writeError(w, http.StatusBadRequest, "keyId is required")
		return
	}

	host, err := s.store.GetHostWithSecrets(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	key, err := s.store.GetKeyWithSecret(body.KeyID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res, err := keys.InjectOnHost(host, key)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	shouldSwitch := true
	if body.SwitchHostAuth != nil {
		shouldSwitch = *body.SwitchHostAuth
	}
	switched := false
	if shouldSwitch && res.AuthWorks {
		if err := s.store.SetHostKeyAuth(id, key.ID); err == nil {
			switched = true
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"result":   res,
		"switched": switched,
	})
}


func (s *Server) handleListInvoices(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("id")
	list, err := s.store.ListInvoices(hostID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invoices": list})
}

func (s *Server) handleUploadInvoice(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("id")
	if _, err := s.store.GetHostWithSecrets(hostID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	invID := uuid.NewString()
	safeName := filepath.Base(strings.TrimSpace(header.Filename))
	if safeName == "" || safeName == "." {
		safeName = "invoice"
	}
	relPath := filepath.Join(hostID, invID+"_"+safeName)
	fullDir := filepath.Join(s.dataDir, "invoices", hostID)
	if err := os.MkdirAll(fullDir, 0700); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fullPath := filepath.Join(s.dataDir, "invoices", relPath)
	dst, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	written, copyErr := io.Copy(dst, file)
	_ = dst.Close()
	if copyErr != nil {
		_ = os.Remove(fullPath)
		writeError(w, http.StatusInternalServerError, copyErr.Error())
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	amount, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("amount")), 64)
	currency := strings.TrimSpace(r.FormValue("currency"))
	if currency == "" {
		currency = "USD"
	}

	created, err := s.store.CreateInvoice(store.Invoice{
		ID:          invID,
		HostID:      hostID,
		Title:       r.FormValue("title"),
		Filename:    safeName,
		StoragePath: relPath,
		MimeType:    mimeType,
		Size:        written,
		Amount:      amount,
		Currency:    currency,
		BilledAt:    strings.TrimSpace(r.FormValue("billedAt")),
		Notes:       r.FormValue("notes"),
	})
	if err != nil {
		_ = os.Remove(fullPath)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetInvoiceFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inv, err := s.store.GetInvoice(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	fullPath := filepath.Join(s.dataDir, "invoices", inv.StoragePath)
	disp := "inline"
	if r.URL.Query().Get("download") == "1" {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disp, strings.ReplaceAll(inv.Filename, `"`, "")))
	if inv.MimeType != "" {
		w.Header().Set("Content-Type", inv.MimeType)
	}
	http.ServeFile(w, r, fullPath)
}

func (s *Server) handleDeleteInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inv, err := s.store.DeleteInvoice(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = os.Remove(filepath.Join(s.dataDir, "invoices", inv.StoragePath))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}


func (s *Server) handleImportHosts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Format  string `json:"format"`
		Content string `json:"content"`
		KeyID   string `json:"keyId"`
		DryRun  bool   `json:"dryRun"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	parsed, detectedFmt, err := importer.ParseAuto(body.Format, body.Content)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if body.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{
			"format": detectedFmt,
			"count":  len(parsed),
			"hosts":  parsed,
		})
		return
	}

	existing, err := s.store.ListHosts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	seen := map[string]bool{}
	for _, h := range existing {
		seen[fmt.Sprintf("%s:%d:%s", strings.ToLower(h.Host), h.Port, h.Username)] = true
	}

	imported := 0
	skipped := 0
	for _, item := range parsed {
		if item.Port <= 0 {
			item.Port = 22
		}
		if item.Username == "" {
			item.Username = "root"
		}
		sig := fmt.Sprintf("%s:%d:%s", strings.ToLower(item.Host), item.Port, item.Username)
		if seen[sig] {
			skipped++
			continue
		}
		if body.KeyID != "" && item.AuthType == "key" {
			item.KeyID = body.KeyID
		}
		if _, err := s.store.CreateHost(item); err == nil {
			imported++
			seen[sig] = true
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"format":   detectedFmt,
		"total":    len(parsed),
		"imported": imported,
		"skipped":  skipped,
	})
}


const tailscaleAuthKeySetting = "tailscale_auth_key"

func (s *Server) handleGetTailscaleSetting(w http.ResponseWriter, r *http.Request) {
	val, err := s.store.GetSetting(tailscaleAuthKeySetting)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"hasAuthKey": val != ""})
}

func (s *Server) handleSaveTailscaleSetting(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AuthKey string `json:"authKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if err := s.store.SetSetting(tailscaleAuthKeySetting, strings.TrimSpace(body.AuthKey)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleCheckTailscale(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	host, err := s.store.GetHostWithSecrets(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	status, err := tailscale.CheckStatus(host)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	synced := false
	if status.Running && status.TailscaleIP != "" {
		if _, err := s.store.UpsertHostEndpoint(id, store.HostEndpoint{
			Label: "Tailscale",
			Host:  status.TailscaleIP,
			Port:  host.Port,
		}); err == nil {
			synced = true
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": status,
		"synced": synced,
	})
}

func (s *Server) handleInjectTailscale(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		AuthKey     string `json:"authKey"`
		SaveAuthKey bool   `json:"saveAuthKey"`
		Hostname    string `json:"hostname"`
		EnableSSH   bool   `json:"enableSSH"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	host, err := s.store.GetHostWithSecrets(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	authKey := strings.TrimSpace(body.AuthKey)
	if authKey == "" {
		authKey, _ = s.store.GetSetting(tailscaleAuthKeySetting)
	}
	if authKey == "" {
		writeError(w, http.StatusBadRequest, "请提供 Tailscale Auth Key (tskey-auth-...)")
		return
	}
	if body.SaveAuthKey && strings.TrimSpace(body.AuthKey) != "" {
		_ = s.store.SetSetting(tailscaleAuthKeySetting, authKey)
	}

	status, err := tailscale.InstallAndUp(host, authKey, body.Hostname, body.EnableSSH)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	synced := false
	if status.TailscaleIP != "" {
		if _, err := s.store.UpsertHostEndpoint(id, store.HostEndpoint{
			Label: "Tailscale",
			Host:  status.TailscaleIP,
			Port:  host.Port,
		}); err == nil {
			synced = true
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": status,
		"synced": synced,
	})
}


func (s *Server) handleExportBackup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Passphrase) == "" {
		writeError(w, http.StatusBadRequest, "passphrase is required")
		return
	}

	data, err := backup.ExportEncrypted(s.store, s.dataDir, body.Passphrase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	filename := fmt.Sprintf("vpscoop-backup-%s.vpscoop", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Server) handleImportBackup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
		Content    string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if strings.TrimSpace(body.Passphrase) == "" || strings.TrimSpace(body.Content) == "" {
		writeError(w, http.StatusBadRequest, "passphrase and backup content are required")
		return
	}

	summary, err := backup.ImportEncrypted(s.store, s.dataDir, []byte(body.Content), body.Passphrase)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}


const webdavConfigSetting = "webdav_config"

func (s *Server) loadWebDAVConfig() (webdav.Config, error) {
	var cfg webdav.Config
	raw, err := s.store.GetSetting(webdavConfigSetting)
	if err != nil || raw == "" {
		return cfg, err
	}
	_ = json.Unmarshal([]byte(raw), &cfg)
	return cfg, nil
}

func (s *Server) saveWebDAVConfig(cfg webdav.Config) error {
	b, _ := json.Marshal(cfg)
	return s.store.SetSetting(webdavConfigSetting, string(b))
}

func (s *Server) handleGetWebDAV(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadWebDAVConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":           cfg.URL,
		"username":      cfg.Username,
		"hasPassword":   cfg.Password != "",
		"hasPassphrase": cfg.Passphrase != "",
		"autoSync":      cfg.AutoSync,
	})
}

func (s *Server) handleSaveWebDAV(w http.ResponseWriter, r *http.Request) {
	var in webdav.Config
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	existing, _ := s.loadWebDAVConfig()
	if in.Password == "" {
		in.Password = existing.Password
	}
	if in.Passphrase == "" {
		in.Passphrase = existing.Passphrase
	}
	in.URL = strings.TrimSpace(in.URL)
	in.Username = strings.TrimSpace(in.Username)

	if err := s.saveWebDAVConfig(in); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleTestWebDAV(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadWebDAVConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var override webdav.Config
	if json.NewDecoder(r.Body).Decode(&override) == nil {
		if strings.TrimSpace(override.URL) != "" {
			cfg.URL = strings.TrimSpace(override.URL)
		}
		if strings.TrimSpace(override.Username) != "" {
			cfg.Username = strings.TrimSpace(override.Username)
		}
		if override.Password != "" {
			cfg.Password = override.Password
		}
	}
	if err := webdav.Test(cfg); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleListWebDAV(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadWebDAVConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	files, err := webdav.List(cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (s *Server) handlePushWebDAV(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadWebDAVConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body struct {
		Passphrase   string `json:"passphrase"`
		KeepSnapshot bool   `json:"keepSnapshot"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	pass := strings.TrimSpace(body.Passphrase)
	if pass == "" {
		pass = cfg.Passphrase
	}
	if pass == "" {
		writeError(w, http.StatusBadRequest, "请先设置用于端到端加密的备份密钥")
		return
	}

	data, err := backup.ExportEncrypted(s.store, s.dataDir, pass)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := webdav.Upload(cfg, "vpscoop-latest.vpscoop", data); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	snapshotName := ""
	if body.KeepSnapshot {
		snapshotName = fmt.Sprintf("vpscoop-%s.vpscoop", time.Now().Format("20060102-150405"))
		_ = webdav.Upload(cfg, snapshotName, data)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"latest":   "vpscoop-latest.vpscoop",
		"snapshot": snapshotName,
		"bytes":    len(data),
	})
}

func (s *Server) handlePullWebDAV(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadWebDAVConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body struct {
		Filename   string `json:"filename"`
		Passphrase string `json:"passphrase"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	filename := strings.TrimSpace(body.Filename)
	if filename == "" {
		filename = "vpscoop-latest.vpscoop"
	}
	pass := strings.TrimSpace(body.Passphrase)
	if pass == "" {
		pass = cfg.Passphrase
	}
	if pass == "" {
		writeError(w, http.StatusBadRequest, "请提供解密密钥")
		return
	}

	raw, err := webdav.Download(cfg, filename)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	summary, err := backup.ImportEncrypted(s.store, s.dataDir, raw, pass)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}


func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	unlocked := s.store != nil
	s.mu.RUnlock()

	initialized := unlocked
	if !initialized {
		dbPath := filepath.Join(s.dataDir, "vpscoop.db")
		salt, verifier, _ := store.ReadMasterMeta(dbPath)
		initialized = (salt != "" && verifier != "") || crypto.HasLegacyKey(s.dataDir)
	}
	authenticated := unlocked && s.isAuthenticated(r)
	writeJSON(w, http.StatusOK, map[string]bool{
		"initialized":   initialized,
		"unlocked":      unlocked,
		"authenticated": authenticated,
	})
}

func (s *Server) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		writeError(w, http.StatusConflict, "主加密密钥已初始化")
		return
	}
	dbPath := filepath.Join(s.dataDir, "vpscoop.db")
	if salt, verifier, _ := store.ReadMasterMeta(dbPath); salt != "" && verifier != "" {
		writeError(w, http.StatusConflict, "数据库已初始化主密钥，请直接解锁")
		return
	}

	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	salt, err := crypto.GenerateSalt()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cipher, err := crypto.DeriveCipher(body.Passphrase, salt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	verifier, err := cipher.Encrypt("vpscoop-ok")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	st, err := store.New(dbPath, cipher)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := st.WriteMasterMeta(base64.StdEncoding.EncodeToString(salt), verifier); err != nil {
		_ = st.Close()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.store = st
	s.issueSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "initialized": true, "unlocked": true, "authenticated": true})
}

func (s *Server) handleAuthUnlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	dbPath := filepath.Join(s.dataDir, "vpscoop.db")
	saltB64, verifier, err := store.ReadMasterMeta(dbPath)
	if err != nil || saltB64 == "" || verifier == "" {
		writeError(w, http.StatusBadRequest, "数据库尚未设置主密钥")
		return
	}
	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid master salt")
		return
	}
	cipher, err := crypto.DeriveCipher(body.Passphrase, salt)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	plain, err := cipher.Decrypt(verifier)
	if err != nil || plain != "vpscoop-ok" {
		writeError(w, http.StatusUnauthorized, "主密钥错误，无法解密数据库")
		return
	}

	s.mu.Lock()
	if s.store == nil {
		st, err := store.New(dbPath, cipher)
		if err != nil {
			s.mu.Unlock()
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.store = st
	}
	s.mu.Unlock()

	s.issueSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "initialized": true, "unlocked": true, "authenticated": true})
}

func (s *Server) handleAuthLock(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.store != nil {
		_ = s.store.Close()
		s.store = nil
	}
	// Rotate sessionSecret so ALL existing browser sessions are immediately invalidated
	sec := make([]byte, 32)
	_, _ = rand.Read(sec)
	s.sessionSecret = sec
	s.mu.Unlock()

	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "unlocked": false, "authenticated": false})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "authenticated": false})
}
