package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"vpscoop/internal/keys"
	"vpscoop/internal/scp"
	sshclient "vpscoop/internal/ssh"
	"vpscoop/internal/store"
	"vpscoop/internal/tailscale"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Server implements a Model Context Protocol (MCP) stdio server for VPSCoop.
type Server struct {
	store *store.Store
}

// NewServer creates an MCP Server backed by st.
func NewServer(st *store.Store) *Server {
	return &Server{store: st}
}

// ServeStdio reads JSON-RPC requests from stdin and writes responses to stdout.
func (s *Server) ServeStdio() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	enc := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		// Notifications (no ID) do not expect a response
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue
		}

		resp := s.handleRequest(req)
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) handleRequest(req rpcRequest) rpcResponse {
	switch req.Method {
	case "initialize":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "vpscoop-mcp",
					"version": "1.0.0",
				},
			},
		}
	case "ping":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": s.tools()},
		}
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &rpcError{Code: -32602, Message: "invalid tools/call params"},
			}
		}
		text, isErr := s.callTool(params.Name, params.Arguments)
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": text},
				},
				"isError": isErr,
			},
		}
	default:
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "method not found: " + req.Method},
		}
	}
}

func (s *Server) tools() []toolDef {
	return []toolDef{
		{
			Name:        "list_hosts",
			Description: "List all VPS hosts managed by VPSCoop (includes provider, region, cost, billingCycle, expiresAt, specs, tags; secrets are redacted).",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "list_expiring_hosts",
			Description: "List VPS hosts that are expired or expiring within a given number of days.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"withinDays": map[string]any{"type": "integer", "description": "Days threshold (default 30)"},
				},
			},
		},
		{
			Name:        "add_host",
			Description: "Add a new VPS host to VPSCoop.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":         map[string]any{"type": "string"},
					"host":         map[string]any{"type": "string", "description": "IP or domain"},
					"port":         map[string]any{"type": "integer", "description": "SSH port (default 22)"},
					"username":     map[string]any{"type": "string", "description": "SSH user (default root)"},
					"authType":     map[string]any{"type": "string", "enum": []string{"password", "key"}},
					"keyId":        map[string]any{"type": "string", "description": "Optional Key Vault ID when authType=key"},
					"password":     map[string]any{"type": "string"},
					"provider":     map[string]any{"type": "string"},
					"region":       map[string]any{"type": "string"},
					"groupName":    map[string]any{"type": "string"},
					"cost":         map[string]any{"type": "number"},
					"currency":     map[string]any{"type": "string", "description": "USD, CNY, EUR, GBP, JPY"},
					"billingCycle": map[string]any{"type": "string", "description": "monthly, quarterly, semiannual, annual, biennial, triennial, onetime"},
					"expiresAt":    map[string]any{"type": "string", "description": "YYYY-MM-DD"},
					"specs":        map[string]any{"type": "string"},
					"notes":        map[string]any{"type": "string"},
				},
				"required": []string{"name", "host"},
			},
		},
		{
			Name:        "delete_host",
			Description: "Delete a VPS host by its ID.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostId": map[string]any{"type": "string"},
				},
				"required": []string{"hostId"},
			},
		},
		{
			Name:        "test_host_ssh",
			Description: "Test SSH connectivity to a VPS host and measure connection latency in ms.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostId": map[string]any{"type": "string"},
				},
				"required": []string{"hostId"},
			},
		},
		{
			Name:        "ssh_exec",
			Description: "Execute a shell command on a VPS host via SSH and return stdout, stderr, and exit status.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostId":  map[string]any{"type": "string"},
					"command": map[string]any{"type": "string"},
				},
				"required": []string{"hostId", "command"},
			},
		},
		{
			Name:        "scp_list",
			Description: "List files and directories at a remote path on a VPS host.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostId": map[string]any{"type": "string"},
					"path":   map[string]any{"type": "string", "description": "Remote directory path (default ~)"},
				},
				"required": []string{"hostId"},
			},
		},
		{
			Name:        "scp_read_file",
			Description: "Read a remote text file (up to 512KB) from a VPS host via SCP.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostId": map[string]any{"type": "string"},
					"path":   map[string]any{"type": "string"},
				},
				"required": []string{"hostId", "path"},
			},
		},
		{
			Name:        "scp_write_file",
			Description: "Write or overwrite a remote text file on a VPS host via SCP.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostId":   map[string]any{"type": "string"},
					"dir":      map[string]any{"type": "string"},
					"filename": map[string]any{"type": "string"},
					"content":  map[string]any{"type": "string"},
				},
				"required": []string{"hostId", "dir", "filename", "content"},
			},
		},
		{
			Name:        "list_keys",
			Description: "List all SSH keys stored in the VPSCoop Key Vault.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "generate_key",
			Description: "Generate a new Ed25519 or RSA-4096 SSH keypair and save it in the Key Vault.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":       map[string]any{"type": "string"},
					"algorithm":  map[string]any{"type": "string", "enum": []string{"ed25519", "ecdsa-p256", "ecdsa-p384", "ecdsa-p521", "rsa-2048", "rsa-3072", "rsa-4096"}},
					"comment":    map[string]any{"type": "string"},
					"passphrase": map[string]any{"type": "string"},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "check_host_key",
			Description: "Check if a Key Vault SSH key is present in remote ~/.ssh/authorized_keys and verify key-based SSH login.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostId": map[string]any{"type": "string"},
					"keyId":  map[string]any{"type": "string"},
				},
				"required": []string{"hostId", "keyId"},
			},
		},
		{
			Name:        "inject_host_key",
			Description: "Automatically inject a Key Vault public key into remote ~/.ssh/authorized_keys, verify key login, and optionally switch host auth to that key.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostId":         map[string]any{"type": "string"},
					"keyId":          map[string]any{"type": "string"},
					"switchHostAuth": map[string]any{"type": "boolean", "description": "Switch host auth to this key if login succeeds (default true)"},
				},
				"required": []string{"hostId", "keyId"},
			},
		},
		{
			Name:        "tailscale_check",
			Description: "Check if Tailscale is installed/running on a VPS host and auto-sync its 100.x IP as a Tailscale endpoint.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostId": map[string]any{"type": "string"},
				},
				"required": []string{"hostId"},
			},
		},
		{
			Name:        "tailscale_inject",
			Description: "Install Tailscale (if missing), join the tailnet using an authKey (or saved default), and auto-add its 100.x IP as a Tailscale endpoint.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostId":    map[string]any{"type": "string"},
					"authKey":   map[string]any{"type": "string", "description": "Optional tskey-auth-... (uses saved key if omitted)"},
					"hostname":  map[string]any{"type": "string"},
					"enableSSH": map[string]any{"type": "boolean"},
				},
				"required": []string{"hostId"},
			},
		},
	}
}

func jsonText(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func (s *Server) callTool(name string, rawArgs json.RawMessage) (string, bool) {
	if len(rawArgs) == 0 {
		rawArgs = []byte("{}")
	}

	switch name {
	case "list_hosts":
		hosts, err := s.store.ListHosts()
		if err != nil {
			return err.Error(), true
		}
		return jsonText(hosts), false

	case "list_expiring_hosts":
		var args struct {
			WithinDays int `json:"withinDays"`
		}
		_ = json.Unmarshal(rawArgs, &args)
		if args.WithinDays <= 0 {
			args.WithinDays = 30
		}
		hosts, err := s.store.ListHosts()
		if err != nil {
			return err.Error(), true
		}
		now := time.Now()
		type expItem struct {
			store.Host
			DaysLeft int `json:"daysLeft"`
		}
		var out []expItem
		for _, h := range hosts {
			if h.ExpiresAt == "" {
				continue
			}
			t, err := time.Parse("2006-01-02", h.ExpiresAt)
			if err != nil {
				continue
			}
			days := int(t.Sub(now).Hours()/24) + 1
			if days <= args.WithinDays {
				out = append(out, expItem{Host: h, DaysLeft: days})
			}
		}
		return jsonText(out), false

	case "add_host":
		var in store.HostInput
		if err := json.Unmarshal(rawArgs, &in); err != nil {
			return err.Error(), true
		}
		if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Host) == "" {
			return "name and host are required", true
		}
		created, err := s.store.CreateHost(in)
		if err != nil {
			return err.Error(), true
		}
		return jsonText(created), false

	case "delete_host":
		var args struct {
			HostID string `json:"hostId"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.HostID == "" {
			return "hostId is required", true
		}
		if err := s.store.DeleteHost(args.HostID); err != nil {
			return err.Error(), true
		}
		return `{"ok":true}`, false

	case "test_host_ssh":
		var args struct {
			HostID string `json:"hostId"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.HostID == "" {
			return "hostId is required", true
		}
		host, err := s.store.GetHostWithSecrets(args.HostID)
		if err != nil {
			return err.Error(), true
		}
		start := time.Now()
		client, err := sshclient.Dial(host)
		if err != nil {
			return err.Error(), true
		}
		_ = client.Close()
		s.store.TouchLastConnected(args.HostID)
		return jsonText(map[string]any{"ok": true, "latencyMs": time.Since(start).Milliseconds()}), false

	case "ssh_exec":
		var args struct {
			HostID  string `json:"hostId"`
			Command string `json:"command"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.HostID == "" || args.Command == "" {
			return "hostId and command are required", true
		}
		host, err := s.store.GetHostWithSecrets(args.HostID)
		if err != nil {
			return err.Error(), true
		}
		client, err := sshclient.Dial(host)
		if err != nil {
			return err.Error(), true
		}
		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			return err.Error(), true
		}
		defer session.Close()

		var stdout, stderr bytes.Buffer
		session.Stdout = &stdout
		session.Stderr = &stderr
		runErr := session.Run(args.Command)
		s.store.TouchLastConnected(args.HostID)

		res := map[string]any{
			"stdout": stdout.String(),
			"stderr": stderr.String(),
			"ok":     runErr == nil,
		}
		if runErr != nil {
			res["error"] = runErr.Error()
		}
		return jsonText(res), runErr != nil

	case "scp_list":
		var args struct {
			HostID string `json:"hostId"`
			Path   string `json:"path"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.HostID == "" {
			return "hostId is required", true
		}
		host, err := s.store.GetHostWithSecrets(args.HostID)
		if err != nil {
			return err.Error(), true
		}
		client, err := sshclient.Dial(host)
		if err != nil {
			return err.Error(), true
		}
		defer client.Close()

		entries, resolved, err := scp.ListDir(client, args.Path)
		if err != nil {
			return err.Error(), true
		}
		return jsonText(map[string]any{"path": resolved, "entries": entries}), false

	case "scp_read_file":
		var args struct {
			HostID string `json:"hostId"`
			Path   string `json:"path"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.HostID == "" || args.Path == "" {
			return "hostId and path are required", true
		}
		host, err := s.store.GetHostWithSecrets(args.HostID)
		if err != nil {
			return err.Error(), true
		}
		client, err := sshclient.Dial(host)
		if err != nil {
			return err.Error(), true
		}
		defer client.Close()

		content, err := scp.ReadFileText(client, args.Path, 512*1024)
		if err != nil {
			return err.Error(), true
		}
		return content, false

	case "scp_write_file":
		var args struct {
			HostID   string `json:"hostId"`
			Dir      string `json:"dir"`
			Filename string `json:"filename"`
			Content  string `json:"content"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.HostID == "" || args.Dir == "" || args.Filename == "" {
			return "hostId, dir, and filename are required", true
		}
		host, err := s.store.GetHostWithSecrets(args.HostID)
		if err != nil {
			return err.Error(), true
		}
		client, err := sshclient.Dial(host)
		if err != nil {
			return err.Error(), true
		}
		defer client.Close()

		data := []byte(args.Content)
		if err := scp.Upload(client, args.Dir, args.Filename, int64(len(data)), bytes.NewReader(data)); err != nil {
			return err.Error(), true
		}
		return `{"ok":true}`, false

	case "list_keys":
		list, err := s.store.ListKeys()
		if err != nil {
			return err.Error(), true
		}
		return jsonText(list), false

	case "generate_key":
		var args struct {
			Name       string `json:"name"`
			Algorithm  string `json:"algorithm"`
			Comment    string `json:"comment"`
			Passphrase string `json:"passphrase"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || strings.TrimSpace(args.Name) == "" {
			return "name is required", true
		}
		gen, err := keys.Generate(args.Algorithm, args.Comment, args.Passphrase)
		if err != nil {
			return err.Error(), true
		}
		created, err := s.store.CreateKey(args.Name, gen.Algorithm, gen.PublicKey, gen.PrivateKey, args.Passphrase, gen.Fingerprint, args.Comment)
		if err != nil {
			return err.Error(), true
		}
		return jsonText(created), false

	case "check_host_key":
		var args struct {
			HostID string `json:"hostId"`
			KeyID  string `json:"keyId"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.HostID == "" || args.KeyID == "" {
			return "hostId and keyId are required", true
		}
		host, err := s.store.GetHostWithSecrets(args.HostID)
		if err != nil {
			return err.Error(), true
		}
		key, err := s.store.GetKeyWithSecret(args.KeyID)
		if err != nil {
			return err.Error(), true
		}
		return jsonText(keys.CheckOnHost(host, key)), false

	case "inject_host_key":
		var args struct {
			HostID         string `json:"hostId"`
			KeyID          string `json:"keyId"`
			SwitchHostAuth *bool  `json:"switchHostAuth"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.HostID == "" || args.KeyID == "" {
			return "hostId and keyId are required", true
		}
		host, err := s.store.GetHostWithSecrets(args.HostID)
		if err != nil {
			return err.Error(), true
		}
		key, err := s.store.GetKeyWithSecret(args.KeyID)
		if err != nil {
			return err.Error(), true
		}
		res, err := keys.InjectOnHost(host, key)
		if err != nil {
			return err.Error(), true
		}
		shouldSwitch := true
		if args.SwitchHostAuth != nil {
			shouldSwitch = *args.SwitchHostAuth
		}
		switched := false
		if shouldSwitch && res.AuthWorks {
			if err := s.store.SetHostKeyAuth(args.HostID, key.ID); err == nil {
				switched = true
			}
		}
		return jsonText(map[string]any{"result": res, "switched": switched}), false

	case "tailscale_check":
		var args struct {
			HostID string `json:"hostId"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.HostID == "" {
			return "hostId is required", true
		}
		host, err := s.store.GetHostWithSecrets(args.HostID)
		if err != nil {
			return err.Error(), true
		}
		status, err := tailscale.CheckStatus(host)
		if err != nil {
			return err.Error(), true
		}
		synced := false
		if status.Running && status.TailscaleIP != "" {
			if _, err := s.store.UpsertHostEndpoint(args.HostID, store.HostEndpoint{
				Label: "Tailscale",
				Host:  status.TailscaleIP,
				Port:  host.Port,
			}); err == nil {
				synced = true
			}
		}
		return jsonText(map[string]any{"status": status, "synced": synced}), false

	case "tailscale_inject":
		var args struct {
			HostID    string `json:"hostId"`
			AuthKey   string `json:"authKey"`
			Hostname  string `json:"hostname"`
			EnableSSH bool   `json:"enableSSH"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.HostID == "" {
			return "hostId is required", true
		}
		host, err := s.store.GetHostWithSecrets(args.HostID)
		if err != nil {
			return err.Error(), true
		}
		authKey := strings.TrimSpace(args.AuthKey)
		if authKey == "" {
			authKey, _ = s.store.GetSetting("tailscale_auth_key")
		}
		if authKey == "" {
			return "authKey is required (none saved)", true
		}
		status, err := tailscale.InstallAndUp(host, authKey, args.Hostname, args.EnableSSH)
		if err != nil {
			return err.Error(), true
		}
		synced := false
		if status.TailscaleIP != "" {
			if _, err := s.store.UpsertHostEndpoint(args.HostID, store.HostEndpoint{
				Label: "Tailscale",
				Host:  status.TailscaleIP,
				Port:  host.Port,
			}); err == nil {
				synced = true
			}
		}
		return jsonText(map[string]any{"status": status, "synced": synced}), false

	default:
		return fmt.Sprintf("unknown tool %q", name), true
	}
}

// Keep io import referenced cleanly if needed
var _ = io.EOF
