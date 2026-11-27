package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"vpscoop/internal/keys"
	"vpscoop/internal/scp"
	sshclient "vpscoop/internal/ssh"
	"vpscoop/internal/store"
	"vpscoop/internal/tailscale"
)

func jsonText(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func textResult(text string) (*sdkmcp.CallToolResult, any, error) {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: text},
		},
	}, nil, nil
}

func errResult(msg string) (*sdkmcp.CallToolResult, any, error) {
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: msg},
		},
	}, nil, nil
}

// NewSDKServer builds an official MCP SDK Server wired with all VPSCoop tools.
func NewSDKServer(st *store.Store) *sdkmcp.Server {
	s := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "vpscoop-mcp",
		Version: "1.0.0",
	}, nil)

	type emptyArgs struct{}

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "list_hosts",
		Description: "List all VPS hosts managed by VPSCoop (includes provider, region, cost, billingCycle, expiresAt, specs, tags; secrets are redacted).",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, _ emptyArgs) (*sdkmcp.CallToolResult, any, error) {
		hosts, err := st.ListHosts()
		if err != nil {
			return errResult(err.Error())
		}
		return textResult(jsonText(hosts))
	})

	type expiringArgs struct {
		WithinDays int `json:"withinDays,omitempty" jsonschema:"Days threshold (default 30)"`
	}
	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "list_expiring_hosts",
		Description: "List VPS hosts that are expired or expiring within a given number of days.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args expiringArgs) (*sdkmcp.CallToolResult, any, error) {
		daysThreshold := args.WithinDays
		if daysThreshold <= 0 {
			daysThreshold = 30
		}
		hosts, err := st.ListHosts()
		if err != nil {
			return errResult(err.Error())
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
			if days <= daysThreshold {
				out = append(out, expItem{Host: h, DaysLeft: days})
			}
		}
		return textResult(jsonText(out))
	})

	type addHostArgs struct {
		Name         string  `json:"name" jsonschema:"Display name of the VPS"`
		Host         string  `json:"host" jsonschema:"IP address or domain"`
		Port         int     `json:"port,omitempty" jsonschema:"SSH port (default 22)"`
		Username     string  `json:"username,omitempty" jsonschema:"SSH user (default root)"`
		AuthType     string  `json:"authType,omitempty" jsonschema:"password or key"`
		KeyID        string  `json:"keyId,omitempty" jsonschema:"Optional Key Vault ID when authType=key"`
		Password     string  `json:"password,omitempty"`
		Provider     string  `json:"provider,omitempty"`
		Region       string  `json:"region,omitempty"`
		GroupName    string  `json:"groupName,omitempty"`
		Cost         float64 `json:"cost,omitempty"`
		Currency     string  `json:"currency,omitempty" jsonschema:"USD, CNY, EUR, GBP, JPY"`
		BillingCycle string  `json:"billingCycle,omitempty" jsonschema:"monthly, quarterly, semiannual, annual, biennial, triennial, onetime"`
		ExpiresAt    string  `json:"expiresAt,omitempty" jsonschema:"YYYY-MM-DD"`
		Specs        string  `json:"specs,omitempty"`
		Notes        string  `json:"notes,omitempty"`
	}
	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "add_host",
		Description: "Add a new VPS host to VPSCoop.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args addHostArgs) (*sdkmcp.CallToolResult, any, error) {
		if strings.TrimSpace(args.Name) == "" || strings.TrimSpace(args.Host) == "" {
			return errResult("name and host are required")
		}
		created, err := st.CreateHost(store.HostInput{
			Name:         args.Name,
			Host:         args.Host,
			Port:         args.Port,
			Username:     args.Username,
			AuthType:     args.AuthType,
			KeyID:        args.KeyID,
			Password:     args.Password,
			Provider:     args.Provider,
			Region:       args.Region,
			GroupName:    args.GroupName,
			Cost:         args.Cost,
			Currency:     args.Currency,
			BillingCycle: args.BillingCycle,
			ExpiresAt:    args.ExpiresAt,
			Specs:        args.Specs,
			Notes:        args.Notes,
		})
		if err != nil {
			return errResult(err.Error())
		}
		return textResult(jsonText(created))
	})

	type hostIDArgs struct {
		HostID string `json:"hostId" jsonschema:"Host ID"`
	}
	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "delete_host",
		Description: "Delete a VPS host by its ID.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args hostIDArgs) (*sdkmcp.CallToolResult, any, error) {
		if args.HostID == "" {
			return errResult("hostId is required")
		}
		if err := st.DeleteHost(args.HostID); err != nil {
			return errResult(err.Error())
		}
		return textResult(`{"ok":true}`)
	})

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "test_host_ssh",
		Description: "Test SSH connectivity to a VPS host and measure connection latency in ms.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args hostIDArgs) (*sdkmcp.CallToolResult, any, error) {
		host, err := st.GetHostWithSecrets(args.HostID)
		if err != nil {
			return errResult(err.Error())
		}
		start := time.Now()
		client, err := sshclient.Dial(host)
		if err != nil {
			return errResult(err.Error())
		}
		_ = client.Close()
		st.TouchLastConnected(args.HostID)
		return textResult(jsonText(map[string]any{"ok": true, "latencyMs": time.Since(start).Milliseconds()}))
	})

	type sshExecArgs struct {
		HostID  string `json:"hostId" jsonschema:"Host ID"`
		Command string `json:"command" jsonschema:"Shell command to execute"`
	}
	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "ssh_exec",
		Description: "Execute a shell command on a VPS host via SSH and return stdout, stderr, and exit status.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args sshExecArgs) (*sdkmcp.CallToolResult, any, error) {
		if args.HostID == "" || args.Command == "" {
			return errResult("hostId and command are required")
		}
		host, err := st.GetHostWithSecrets(args.HostID)
		if err != nil {
			return errResult(err.Error())
		}
		client, err := sshclient.Dial(host)
		if err != nil {
			return errResult(err.Error())
		}
		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			return errResult(err.Error())
		}
		defer session.Close()

		var stdout, stderr bytes.Buffer
		session.Stdout = &stdout
		session.Stderr = &stderr
		runErr := session.Run(args.Command)
		st.TouchLastConnected(args.HostID)

		res := map[string]any{
			"stdout": stdout.String(),
			"stderr": stderr.String(),
			"ok":     runErr == nil,
		}
		if runErr != nil {
			res["error"] = runErr.Error()
			return &sdkmcp.CallToolResult{
				IsError: true,
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: jsonText(res)}},
			}, nil, nil
		}
		return textResult(jsonText(res))
	})

	type scpListArgs struct {
		HostID string `json:"hostId" jsonschema:"Host ID"`
		Path   string `json:"path,omitempty" jsonschema:"Remote directory path (default ~)"`
	}
	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "scp_list",
		Description: "List files and directories at a remote path on a VPS host.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args scpListArgs) (*sdkmcp.CallToolResult, any, error) {
		host, err := st.GetHostWithSecrets(args.HostID)
		if err != nil {
			return errResult(err.Error())
		}
		client, err := sshclient.Dial(host)
		if err != nil {
			return errResult(err.Error())
		}
		defer client.Close()

		entries, resolved, err := scp.ListDir(client, args.Path)
		if err != nil {
			return errResult(err.Error())
		}
		return textResult(jsonText(map[string]any{"path": resolved, "entries": entries}))
	})

	type scpReadArgs struct {
		HostID string `json:"hostId" jsonschema:"Host ID"`
		Path   string `json:"path" jsonschema:"Remote file path"`
	}
	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "scp_read_file",
		Description: "Read a remote text file (up to 512KB) from a VPS host via SCP.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args scpReadArgs) (*sdkmcp.CallToolResult, any, error) {
		host, err := st.GetHostWithSecrets(args.HostID)
		if err != nil {
			return errResult(err.Error())
		}
		client, err := sshclient.Dial(host)
		if err != nil {
			return errResult(err.Error())
		}
		defer client.Close()

		content, err := scp.ReadFileText(client, args.Path, 512*1024)
		if err != nil {
			return errResult(err.Error())
		}
		return textResult(content)
	})

	type scpWriteArgs struct {
		HostID   string `json:"hostId" jsonschema:"Host ID"`
		Dir      string `json:"dir" jsonschema:"Remote directory"`
		Filename string `json:"filename" jsonschema:"File name"`
		Content  string `json:"content" jsonschema:"Text content to write"`
	}
	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "scp_write_file",
		Description: "Write or overwrite a remote text file on a VPS host via SCP.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args scpWriteArgs) (*sdkmcp.CallToolResult, any, error) {
		host, err := st.GetHostWithSecrets(args.HostID)
		if err != nil {
			return errResult(err.Error())
		}
		client, err := sshclient.Dial(host)
		if err != nil {
			return errResult(err.Error())
		}
		defer client.Close()

		data := []byte(args.Content)
		if err := scp.Upload(client, args.Dir, args.Filename, int64(len(data)), bytes.NewReader(data)); err != nil {
			return errResult(err.Error())
		}
		return textResult(`{"ok":true}`)
	})

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "list_keys",
		Description: "List all SSH keys stored in the VPSCoop Key Vault.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, _ emptyArgs) (*sdkmcp.CallToolResult, any, error) {
		list, err := st.ListKeys()
		if err != nil {
			return errResult(err.Error())
		}
		return textResult(jsonText(list))
	})

	type genKeyArgs struct {
		Name       string `json:"name" jsonschema:"Key name"`
		Algorithm  string `json:"algorithm,omitempty" jsonschema:"ed25519, ecdsa-p256, ecdsa-p384, ecdsa-p521, rsa-2048, rsa-3072, rsa-4096"`
		Comment    string `json:"comment,omitempty"`
		Passphrase string `json:"passphrase,omitempty"`
	}
	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "generate_key",
		Description: "Generate a new SSH keypair and save it in the Key Vault.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args genKeyArgs) (*sdkmcp.CallToolResult, any, error) {
		if strings.TrimSpace(args.Name) == "" {
			return errResult("name is required")
		}
		gen, err := keys.Generate(args.Algorithm, args.Comment, args.Passphrase)
		if err != nil {
			return errResult(err.Error())
		}
		created, err := st.CreateKey(args.Name, gen.Algorithm, gen.PublicKey, gen.PrivateKey, args.Passphrase, gen.Fingerprint, args.Comment)
		if err != nil {
			return errResult(err.Error())
		}
		return textResult(jsonText(created))
	})

	type keyOpArgs struct {
		HostID         string `json:"hostId" jsonschema:"Host ID"`
		KeyID          string `json:"keyId" jsonschema:"Key Vault ID"`
		SwitchHostAuth *bool  `json:"switchHostAuth,omitempty" jsonschema:"Switch host auth to this key if login succeeds (default true)"`
	}
	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "check_host_key",
		Description: "Check if a Key Vault SSH key is present in remote ~/.ssh/authorized_keys and verify key-based SSH login.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args keyOpArgs) (*sdkmcp.CallToolResult, any, error) {
		host, err := st.GetHostWithSecrets(args.HostID)
		if err != nil {
			return errResult(err.Error())
		}
		key, err := st.GetKeyWithSecret(args.KeyID)
		if err != nil {
			return errResult(err.Error())
		}
		return textResult(jsonText(keys.CheckOnHost(host, key)))
	})

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "inject_host_key",
		Description: "Automatically inject a Key Vault public key into remote ~/.ssh/authorized_keys, verify key login, and optionally switch host auth to that key.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args keyOpArgs) (*sdkmcp.CallToolResult, any, error) {
		host, err := st.GetHostWithSecrets(args.HostID)
		if err != nil {
			return errResult(err.Error())
		}
		key, err := st.GetKeyWithSecret(args.KeyID)
		if err != nil {
			return errResult(err.Error())
		}
		res, err := keys.InjectOnHost(host, key)
		if err != nil {
			return errResult(err.Error())
		}
		shouldSwitch := true
		if args.SwitchHostAuth != nil {
			shouldSwitch = *args.SwitchHostAuth
		}
		switched := false
		if shouldSwitch && res.AuthWorks {
			if err := st.SetHostKeyAuth(args.HostID, key.ID); err == nil {
				switched = true
			}
		}
		return textResult(jsonText(map[string]any{"result": res, "switched": switched}))
	})

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "tailscale_check",
		Description: "Check if Tailscale is installed/running on a VPS host and auto-sync its 100.x IP as a Tailscale endpoint.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args hostIDArgs) (*sdkmcp.CallToolResult, any, error) {
		host, err := st.GetHostWithSecrets(args.HostID)
		if err != nil {
			return errResult(err.Error())
		}
		status, err := tailscale.CheckStatus(host)
		if err != nil {
			return errResult(err.Error())
		}
		synced := false
		if status.Running && status.TailscaleIP != "" {
			if _, err := st.UpsertHostEndpoint(args.HostID, store.HostEndpoint{
				Label: "Tailscale",
				Host:  status.TailscaleIP,
				Port:  host.Port,
			}); err == nil {
				synced = true
			}
		}
		return textResult(jsonText(map[string]any{"status": status, "synced": synced}))
	})

	type tailscaleInjectArgs struct {
		HostID    string `json:"hostId" jsonschema:"Host ID"`
		AuthKey   string `json:"authKey,omitempty" jsonschema:"Optional tskey-auth-... (uses saved key if omitted)"`
		Hostname  string `json:"hostname,omitempty"`
		EnableSSH bool   `json:"enableSSH,omitempty"`
	}
	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "tailscale_inject",
		Description: "Install Tailscale (if missing), join the tailnet using an authKey (or saved default), and auto-add its 100.x IP as a Tailscale endpoint.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args tailscaleInjectArgs) (*sdkmcp.CallToolResult, any, error) {
		host, err := st.GetHostWithSecrets(args.HostID)
		if err != nil {
			return errResult(err.Error())
		}
		authKey := strings.TrimSpace(args.AuthKey)
		if authKey == "" {
			authKey, _ = st.GetSetting("tailscale_auth_key")
		}
		if authKey == "" {
			return errResult("authKey is required (none saved)")
		}
		status, err := tailscale.InstallAndUp(host, authKey, args.Hostname, args.EnableSSH)
		if err != nil {
			return errResult(err.Error())
		}
		synced := false
		if status.TailscaleIP != "" {
			if _, err := st.UpsertHostEndpoint(args.HostID, store.HostEndpoint{
				Label: "Tailscale",
				Host:  status.TailscaleIP,
				Port:  host.Port,
			}); err == nil {
				synced = true
			}
		}
		return textResult(jsonText(map[string]any{"status": status, "synced": synced}))
	})

	return s
}

// ServeStdio runs the MCP server over stdin/stdout using the official Go MCP SDK.
func ServeStdio(st *store.Store) error {
	return NewSDKServer(st).Run(context.Background(), &sdkmcp.StdioTransport{})
}

// NewHTTPHandler returns a Streamable HTTP MCP handler backed by the active store.
func NewHTTPHandler(getStore func() *store.Store) http.Handler {
	return sdkmcp.NewStreamableHTTPHandler(func(r *http.Request) *sdkmcp.Server {
		st := getStore()
		if st == nil {
			return nil
		}
		return NewSDKServer(st)
	}, &sdkmcp.StreamableHTTPOptions{
		DisableLocalhostProtection: true,
	})
}

var _ = fmt.Sprintf
