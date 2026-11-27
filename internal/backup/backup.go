package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/pbkdf2"

	"vpscoop/internal/store"
)

const (
	backupFormat     = "vpscoop-encrypted-backup"
	backupVersion    = 1
	pbkdf2Iterations = 100000
)

// Envelope is the encrypted portable backup file structure.
type Envelope struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type exportedHost struct {
	OldID        string               `json:"oldId"`
	Name         string               `json:"name"`
	GroupName    string               `json:"groupName"`
	Provider     string               `json:"provider"`
	Region       string               `json:"region"`
	Host         string               `json:"host"`
	Port         int                  `json:"port"`
	Endpoints    []store.HostEndpoint `json:"endpoints"`
	Username     string               `json:"username"`
	AuthType     string               `json:"authType"`
	KeyID        string               `json:"keyId"`
	Password     string               `json:"password"`
	PrivateKey   string               `json:"privateKey"`
	Passphrase   string               `json:"passphrase"`
	Cost         float64              `json:"cost"`
	Currency     string               `json:"currency"`
	BillingCycle string               `json:"billingCycle"`
	ExpiresAt    string               `json:"expiresAt"`
	PurchasedAt  string               `json:"purchasedAt"`
	Specs        string               `json:"specs"`
	Tags         []string             `json:"tags"`
	Notes        string               `json:"notes"`
}

type exportedKey struct {
	OldID       string `json:"oldId"`
	Name        string `json:"name"`
	Algorithm   string `json:"algorithm"`
	PublicKey   string `json:"publicKey"`
	PrivateKey  string `json:"privateKey"`
	Passphrase  string `json:"passphrase"`
	Fingerprint string `json:"fingerprint"`
	Comment     string `json:"comment"`
}

type exportedInvoice struct {
	OldHostID  string  `json:"oldHostId"`
	Title      string  `json:"title"`
	Filename   string  `json:"filename"`
	MimeType   string  `json:"mimeType"`
	Size       int64   `json:"size"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	BilledAt   string  `json:"billedAt"`
	Notes      string  `json:"notes"`
	FileBase64 string  `json:"fileBase64,omitempty"`
}

type payload struct {
	ExportedAt       string            `json:"exportedAt"`
	Keys             []exportedKey     `json:"keys"`
	Hosts            []exportedHost    `json:"hosts"`
	Invoices         []exportedInvoice `json:"invoices"`
	TailscaleAuthKey string            `json:"tailscaleAuthKey,omitempty"`
}

// ImportSummary reports counts of restored items.
type ImportSummary struct {
	HostsImported    int `json:"hostsImported"`
	HostsSkipped     int `json:"hostsSkipped"`
	KeysImported     int `json:"keysImported"`
	KeysSkipped      int `json:"keysSkipped"`
	InvoicesImported int `json:"invoicesImported"`
}

// ExportEncrypted exports all hosts, keys, invoices, and settings encrypted with passphrase.
func ExportEncrypted(st *store.Store, dataDir, passphrase string) ([]byte, error) {
	if strings.TrimSpace(passphrase) == "" {
		return nil, errors.New("passphrase is required")
	}

	keyList, err := st.ListKeys()
	if err != nil {
		return nil, err
	}
	keysOut := make([]exportedKey, 0, len(keyList))
	for _, k := range keyList {
		full, err := st.GetKeyWithSecret(k.ID)
		if err != nil {
			return nil, err
		}
		keysOut = append(keysOut, exportedKey{
			OldID:       full.ID,
			Name:        full.Name,
			Algorithm:   full.Algorithm,
			PublicKey:   full.PublicKey,
			PrivateKey:  full.PrivateKey,
			Passphrase:  full.Passphrase,
			Fingerprint: full.Fingerprint,
			Comment:     full.Comment,
		})
	}

	hostList, err := st.ListHosts()
	if err != nil {
		return nil, err
	}
	hostsOut := make([]exportedHost, 0, len(hostList))
	for _, h := range hostList {
		full, err := st.GetHostWithSecrets(h.ID)
		if err != nil {
			return nil, err
		}
		hostsOut = append(hostsOut, exportedHost{
			OldID:        full.ID,
			Name:         full.Name,
			GroupName:    full.GroupName,
			Provider:     full.Provider,
			Region:       full.Region,
			Host:         full.Host.Host,
			Port:         full.Port,
			Endpoints:    full.Endpoints,
			Username:     full.Username,
			AuthType:     full.AuthType,
			KeyID:        full.KeyID,
			Password:     full.Password,
			PrivateKey:   full.PrivateKey,
			Passphrase:   full.Passphrase,
			Cost:         full.Cost,
			Currency:     full.Currency,
			BillingCycle: full.BillingCycle,
			ExpiresAt:    full.ExpiresAt,
			PurchasedAt:  full.PurchasedAt,
			Specs:        full.Specs,
			Tags:         full.Tags,
			Notes:        full.Notes,
		})
	}

	invList, err := st.ListInvoices("")
	if err != nil {
		return nil, err
	}
	invsOut := make([]exportedInvoice, 0, len(invList))
	for _, inv := range invList {
		ei := exportedInvoice{
			OldHostID: inv.HostID,
			Title:     inv.Title,
			Filename:  inv.Filename,
			MimeType:  inv.MimeType,
			Size:      inv.Size,
			Amount:    inv.Amount,
			Currency:  inv.Currency,
			BilledAt:  inv.BilledAt,
			Notes:     inv.Notes,
		}
		if inv.StoragePath != "" {
			if raw, rErr := os.ReadFile(filepath.Join(dataDir, "invoices", inv.StoragePath)); rErr == nil {
				ei.FileBase64 = base64.StdEncoding.EncodeToString(raw)
			}
		}
		invsOut = append(invsOut, ei)
	}

	tsKey, _ := st.GetSetting("tailscale_auth_key")

	p := payload{
		ExportedAt:       time.Now().UTC().Format(time.RFC3339),
		Keys:             keysOut,
		Hosts:            hostsOut,
		Invoices:         invsOut,
		TailscaleAuthKey: tsKey,
	}
	plainBytes, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	derivedKey := pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, 32, sha256.New)

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := aead.Seal(nil, nonce, plainBytes, nil)

	env := Envelope{
		Format:     backupFormat,
		Version:    backupVersion,
		KDF:        "pbkdf2-sha256",
		Iterations: pbkdf2Iterations,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(sealed),
	}
	return json.MarshalIndent(env, "", "  ")
}

// ImportEncrypted decrypts an encrypted backup envelope using passphrase and merges it into st.
func ImportEncrypted(st *store.Store, dataDir string, envelopeBytes []byte, passphrase string) (ImportSummary, error) {
	var summary ImportSummary
	if strings.TrimSpace(passphrase) == "" {
		return summary, errors.New("passphrase is required")
	}

	var env Envelope
	if err := json.Unmarshal(envelopeBytes, &env); err != nil {
		return summary, fmt.Errorf("invalid backup file format: %w", err)
	}
	if env.Format != backupFormat {
		return summary, fmt.Errorf("unsupported backup format %q", env.Format)
	}
	if env.Iterations <= 0 {
		env.Iterations = pbkdf2Iterations
	}

	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return summary, errors.New("invalid backup salt")
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return summary, errors.New("invalid backup nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return summary, errors.New("invalid backup ciphertext")
	}

	derivedKey := pbkdf2.Key([]byte(passphrase), salt, env.Iterations, 32, sha256.New)
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return summary, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return summary, err
	}

	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return summary, errors.New("解密失败：密钥错误或备份文件已损坏")
	}

	var p payload
	if err := json.Unmarshal(plain, &p); err != nil {
		return summary, fmt.Errorf("corrupted backup payload: %w", err)
	}

	// 1. Restore SSH Keys (deduplicate by fingerprint+name)
	existingKeys, _ := st.ListKeys()
	keyBySig := map[string]string{}
	for _, k := range existingKeys {
		keyBySig[k.Fingerprint+"|"+k.Name] = k.ID
	}
	keyIDMap := map[string]string{}

	for _, k := range p.Keys {
		sig := k.Fingerprint + "|" + k.Name
		if existingID, ok := keyBySig[sig]; ok {
			keyIDMap[k.OldID] = existingID
			summary.KeysSkipped++
			continue
		}
		created, err := st.CreateKey(k.Name, k.Algorithm, k.PublicKey, k.PrivateKey, k.Passphrase, k.Fingerprint, k.Comment)
		if err != nil {
			continue
		}
		keyIDMap[k.OldID] = created.ID
		keyBySig[sig] = created.ID
		summary.KeysImported++
	}

	// 2. Restore Hosts (deduplicate by host:port:username)
	existingHosts, _ := st.ListHosts()
	hostBySig := map[string]string{}
	for _, h := range existingHosts {
		hostBySig[fmt.Sprintf("%s:%d:%s", strings.ToLower(h.Host), h.Port, h.Username)] = h.ID
	}
	hostIDMap := map[string]string{}

	for _, h := range p.Hosts {
		sig := fmt.Sprintf("%s:%d:%s", strings.ToLower(h.Host), h.Port, h.Username)
		if existingID, ok := hostBySig[sig]; ok {
			hostIDMap[h.OldID] = existingID
			summary.HostsSkipped++
			continue
		}
		mappedKeyID := h.KeyID
		if mapped, ok := keyIDMap[h.KeyID]; ok {
			mappedKeyID = mapped
		}
		created, err := st.CreateHost(store.HostInput{
			Name:         h.Name,
			GroupName:    h.GroupName,
			Provider:     h.Provider,
			Region:       h.Region,
			Host:         h.Host,
			Port:         h.Port,
			Endpoints:    h.Endpoints,
			Username:     h.Username,
			AuthType:     h.AuthType,
			KeyID:        mappedKeyID,
			Password:     h.Password,
			PrivateKey:   h.PrivateKey,
			Passphrase:   h.Passphrase,
			Cost:         h.Cost,
			Currency:     h.Currency,
			BillingCycle: h.BillingCycle,
			ExpiresAt:    h.ExpiresAt,
			PurchasedAt:  h.PurchasedAt,
			Specs:        h.Specs,
			Tags:         h.Tags,
			Notes:        h.Notes,
		})
		if err != nil {
			continue
		}
		hostIDMap[h.OldID] = created.ID
		hostBySig[sig] = created.ID
		summary.HostsImported++
	}

	// 3. Restore Invoices
	for _, inv := range p.Invoices {
		newHostID, ok := hostIDMap[inv.OldHostID]
		if !ok || newHostID == "" {
			continue
		}
		invID := uuid.NewString()
		safeName := filepath.Base(strings.TrimSpace(inv.Filename))
		if safeName == "" || safeName == "." {
			safeName = "invoice"
		}
		var relPath string
		if inv.FileBase64 != "" {
			if raw, decErr := base64.StdEncoding.DecodeString(inv.FileBase64); decErr == nil {
				relPath = filepath.Join(newHostID, invID+"_"+safeName)
				fullDir := filepath.Join(dataDir, "invoices", newHostID)
				if os.MkdirAll(fullDir, 0700) == nil {
					if os.WriteFile(filepath.Join(dataDir, "invoices", relPath), raw, 0600) == nil {
						inv.Size = int64(len(raw))
					} else {
						relPath = ""
					}
				}
			}
		}
		if _, err := st.CreateInvoice(store.Invoice{
			ID:          invID,
			HostID:      newHostID,
			Title:       inv.Title,
			Filename:    safeName,
			StoragePath: relPath,
			MimeType:    inv.MimeType,
			Size:        inv.Size,
			Amount:      inv.Amount,
			Currency:    inv.Currency,
			BilledAt:    inv.BilledAt,
			Notes:       inv.Notes,
		}); err == nil {
			summary.InvoicesImported++
		}
	}

	// 4. Restore Tailscale Auth Key if present
	if p.TailscaleAuthKey != "" {
		_ = st.SetSetting("tailscale_auth_key", p.TailscaleAuthKey)
	}

	return summary, nil
}
