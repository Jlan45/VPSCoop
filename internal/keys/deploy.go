package keys

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	sshclient "vpscoop/internal/ssh"
	"vpscoop/internal/store"
)

// KeyCheckResult describes whether a key is present in authorized_keys and can authenticate via SSH.
type KeyCheckResult struct {
	KeyID            string `json:"keyId"`
	KeyName          string `json:"keyName"`
	InAuthorizedKeys bool   `json:"inAuthorizedKeys"`
	AuthWorks        bool   `json:"authWorks"`
	LatencyMs        int64  `json:"latencyMs,omitempty"`
	Error            string `json:"error,omitempty"`
}

func publicKeyBlob(pubKey string) string {
	fields := strings.Fields(strings.TrimSpace(pubKey))
	if len(fields) >= 2 {
		return fields[1]
	}
	return strings.TrimSpace(pubKey)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// CheckOnHost checks if key is present in remote ~/.ssh/authorized_keys and tests key-based SSH login.
func CheckOnHost(host store.HostWithSecrets, key store.SSHKeyWithSecret) KeyCheckResult {
	res := KeyCheckResult{
		KeyID:   key.ID,
		KeyName: key.Name,
	}

	blob := publicKeyBlob(key.PublicKey)

	// 1. Check remote authorized_keys using host's configured credentials
	if client, err := sshclient.Dial(host); err == nil {
		defer client.Close()
		if sess, sErr := client.NewSession(); sErr == nil {
			var out bytes.Buffer
			sess.Stdout = &out
			cmd := fmt.Sprintf("grep -qF %s ~/.ssh/authorized_keys 2>/dev/null && echo FOUND || echo MISSING", shellQuote(blob))
			if sess.Run(cmd) == nil && strings.Contains(out.String(), "FOUND") {
				res.InAuthorizedKeys = true
			}
			_ = sess.Close()
		}
	} else {
		res.Error = fmt.Sprintf("current auth dial failed: %v", err)
	}

	// 2. Test direct key-based SSH authentication
	start := time.Now()
	if kClient, err := sshclient.DialWithKey(host, key.PrivateKey, key.Passphrase); err == nil {
		res.AuthWorks = true
		res.LatencyMs = time.Since(start).Milliseconds()
		res.Error = ""
		_ = kClient.Close()
	} else if res.Error == "" {
		res.Error = fmt.Sprintf("key auth failed: %v", err)
	}

	return res
}

// InjectOnHost appends key.PublicKey to remote ~/.ssh/authorized_keys (idempotently) and verifies key login.
func InjectOnHost(host store.HostWithSecrets, key store.SSHKeyWithSecret) (KeyCheckResult, error) {
	res := KeyCheckResult{
		KeyID:   key.ID,
		KeyName: key.Name,
	}

	client, err := sshclient.Dial(host)
	if err != nil {
		return res, fmt.Errorf("connect with current credentials: %w", err)
	}
	defer client.Close()

	blob := publicKeyBlob(key.PublicKey)
	pubLine := strings.TrimSpace(key.PublicKey)
	cmd := fmt.Sprintf(
		"mkdir -p ~/.ssh && chmod 700 ~/.ssh && touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && "+
			"(grep -qF %s ~/.ssh/authorized_keys || printf '\\n%%s\\n' %s >> ~/.ssh/authorized_keys)",
		shellQuote(blob),
		shellQuote(pubLine),
	)

	sess, err := client.NewSession()
	if err != nil {
		return res, err
	}
	var stderr bytes.Buffer
	sess.Stderr = &stderr
	runErr := sess.Run(cmd)
	_ = sess.Close()
	if runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = runErr.Error()
		}
		return res, fmt.Errorf("inject authorized_keys: %s", msg)
	}

	res.InAuthorizedKeys = true

	// Verify key-based login immediately
	start := time.Now()
	kClient, err := sshclient.DialWithKey(host, key.PrivateKey, key.Passphrase)
	if err != nil {
		res.Error = fmt.Sprintf("key injected, but key login test failed: %v", err)
		return res, nil
	}
	_ = kClient.Close()
	res.AuthWorks = true
	res.LatencyMs = time.Since(start).Milliseconds()
	return res, nil
}
