package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"vpscoop/internal/crypto"
)

var ErrNotFound = errors.New("host not found")

// HostEndpoint represents an alternative IP/domain/port login endpoint for a VPS host.
type HostEndpoint struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Host  string `json:"host"`
	Port  int    `json:"port"`
}

// Host represents a VPS host entry returned to the API (secrets redacted).
type Host struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	GroupName       string         `json:"groupName"`
	Provider        string         `json:"provider"`
	Region          string         `json:"region"`
	Host            string         `json:"host"`
	Port            int            `json:"port"`
	Endpoints       []HostEndpoint `json:"endpoints"`
	Username        string   `json:"username"`
	AuthType        string   `json:"authType"` // "password" | "key"
	KeyID           string   `json:"keyId"`
	HasPassword     bool     `json:"hasPassword"`
	HasPrivateKey   bool     `json:"hasPrivateKey"`
	Cost            float64  `json:"cost"`
	Currency        string   `json:"currency"`     // "USD", "CNY", "EUR", "GBP", "JPY"
	BillingCycle    string   `json:"billingCycle"` // "monthly", "quarterly", "semiannual", "annual", "biennial", "triennial", "onetime"
	ExpiresAt       string   `json:"expiresAt"`    // "YYYY-MM-DD"
	PurchasedAt     string   `json:"purchasedAt"`  // "YYYY-MM-DD"
	Specs           string   `json:"specs"`
	Tags            []string `json:"tags"`
	Notes           string   `json:"notes"`
	LastConnectedAt string   `json:"lastConnectedAt"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

// HostInput is the payload for creating or updating a host.
type HostInput struct {
	Name         string   `json:"name"`
	GroupName    string   `json:"groupName"`
	Provider     string   `json:"provider"`
	Region       string   `json:"region"`
	Host         string         `json:"host"`
	Port         int            `json:"port"`
	Endpoints    []HostEndpoint `json:"endpoints"`
	Username     string         `json:"username"`
	AuthType     string   `json:"authType"`
	KeyID        string   `json:"keyId"`
	Password     string   `json:"password"`
	PrivateKey   string   `json:"privateKey"`
	Passphrase   string   `json:"passphrase"`
	Cost         float64  `json:"cost"`
	Currency     string   `json:"currency"`
	BillingCycle string   `json:"billingCycle"`
	ExpiresAt    string   `json:"expiresAt"`
	PurchasedAt  string   `json:"purchasedAt"`
	Specs        string   `json:"specs"`
	Tags         []string `json:"tags"`
	Notes        string   `json:"notes"`
}

// HostWithSecrets includes decrypted SSH credentials for internal connection use.
type HostWithSecrets struct {
	Host
	Password   string
	PrivateKey string
	Passphrase string
}

// Store manages persistence of VPS hosts in SQLite.
type Store struct {
	db     *sql.DB
	cipher *crypto.Cipher
}

// New opens or initializes the SQLite database at dbPath.
func New(dbPath string, cipher *crypto.Cipher) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db, cipher: cipher}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS hosts (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	group_name TEXT NOT NULL DEFAULT '',
	provider TEXT NOT NULL DEFAULT '',
	region TEXT NOT NULL DEFAULT '',
	host TEXT NOT NULL,
	port INTEGER NOT NULL DEFAULT 22,
	username TEXT NOT NULL DEFAULT 'root',
	auth_type TEXT NOT NULL DEFAULT 'password',
	key_id TEXT NOT NULL DEFAULT '',
	password_enc TEXT NOT NULL DEFAULT '',
	private_key_enc TEXT NOT NULL DEFAULT '',
	passphrase_enc TEXT NOT NULL DEFAULT '',
	cost REAL NOT NULL DEFAULT 0,
	currency TEXT NOT NULL DEFAULT 'USD',
	billing_cycle TEXT NOT NULL DEFAULT 'monthly',
	expires_at TEXT NOT NULL DEFAULT '',
	purchased_at TEXT NOT NULL DEFAULT '',
	specs TEXT NOT NULL DEFAULT '',
	tags_json TEXT NOT NULL DEFAULT '[]',
	notes TEXT NOT NULL DEFAULT '',
	last_connected_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ssh_keys (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	algorithm TEXT NOT NULL,
	public_key TEXT NOT NULL,
	private_key_enc TEXT NOT NULL,
	passphrase_enc TEXT NOT NULL DEFAULT '',
	fingerprint TEXT NOT NULL,
	comment TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS invoices (
	id TEXT PRIMARY KEY,
	host_id TEXT NOT NULL,
	title TEXT NOT NULL,
	filename TEXT NOT NULL,
	storage_path TEXT NOT NULL,
	mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
	size INTEGER NOT NULL DEFAULT 0,
	amount REAL NOT NULL DEFAULT 0,
	currency TEXT NOT NULL DEFAULT 'USD',
	billed_at TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value_enc TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Ensure columns exist on pre-existing databases
	_, _ = s.db.Exec(`ALTER TABLE hosts ADD COLUMN key_id TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE hosts ADD COLUMN endpoints_json TEXT NOT NULL DEFAULT '[]'`)
	return nil
}

func normalizeInput(in *HostInput) {
	in.Name = strings.TrimSpace(in.Name)
	in.GroupName = strings.TrimSpace(in.GroupName)
	in.Provider = strings.TrimSpace(in.Provider)
	in.Region = strings.TrimSpace(in.Region)
	in.Host = strings.TrimSpace(in.Host)
	in.Username = strings.TrimSpace(in.Username)
	if in.Port <= 0 || in.Port > 65535 {
		in.Port = 22
	}
	if in.Username == "" {
		in.Username = "root"
	}
	if in.AuthType != "key" {
		in.AuthType = "password"
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}
	if in.BillingCycle == "" {
		in.BillingCycle = "monthly"
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	cleaned := make([]HostEndpoint, 0, len(in.Endpoints))
	for _, ep := range in.Endpoints {
		ep.Host = strings.TrimSpace(ep.Host)
		if ep.Host == "" {
			continue
		}
		if ep.ID == "" {
			ep.ID = uuid.NewString()
		}
		ep.Label = strings.TrimSpace(ep.Label)
		if ep.Port <= 0 || ep.Port > 65535 {
			ep.Port = 22
		}
		cleaned = append(cleaned, ep)
	}
	in.Endpoints = cleaned
}

// ListHosts returns all hosts ordered by name without exposing secret values.
func (s *Store) ListHosts() ([]Host, error) {
	rows, err := s.db.Query(`
		SELECT id, name, group_name, provider, region, host, port, endpoints_json, username, auth_type, key_id,
		       password_enc, private_key_enc, cost, currency, billing_cycle,
		       expires_at, purchased_at, specs, tags_json, notes, last_connected_at,
		       created_at, updated_at
		FROM hosts
		ORDER BY LOWER(name) ASC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hosts := []Host{}
	for rows.Next() {
		var h Host
		var pwdEnc, keyEnc, tagsJSON, endpointsJSON string
		if err := rows.Scan(
			&h.ID, &h.Name, &h.GroupName, &h.Provider, &h.Region, &h.Host, &h.Port, &endpointsJSON,
			&h.Username, &h.AuthType, &h.KeyID, &pwdEnc, &keyEnc, &h.Cost, &h.Currency,
			&h.BillingCycle, &h.ExpiresAt, &h.PurchasedAt, &h.Specs, &tagsJSON,
			&h.Notes, &h.LastConnectedAt, &h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			return nil, err
		}
		h.HasPassword = pwdEnc != ""
		h.HasPrivateKey = keyEnc != "" || h.KeyID != ""
		if err := json.Unmarshal([]byte(tagsJSON), &h.Tags); err != nil || h.Tags == nil {
			h.Tags = []string{}
		}
		if err := json.Unmarshal([]byte(endpointsJSON), &h.Endpoints); err != nil || h.Endpoints == nil {
			h.Endpoints = []HostEndpoint{}
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

// GetHostWithSecrets returns a host and its decrypted SSH credentials.
func (s *Store) GetHostWithSecrets(id string) (HostWithSecrets, error) {
	var res HostWithSecrets
	var pwdEnc, keyEnc, passEnc, tagsJSON, endpointsJSON string
	err := s.db.QueryRow(`
		SELECT id, name, group_name, provider, region, host, port, endpoints_json, username, auth_type, key_id,
		       password_enc, private_key_enc, passphrase_enc, cost, currency, billing_cycle,
		       expires_at, purchased_at, specs, tags_json, notes, last_connected_at,
		       created_at, updated_at
		FROM hosts WHERE id = ?`, id).Scan(
		&res.ID, &res.Name, &res.GroupName, &res.Provider, &res.Region, &res.Host.Host, &res.Port, &endpointsJSON,
		&res.Username, &res.AuthType, &res.KeyID, &pwdEnc, &keyEnc, &passEnc, &res.Cost, &res.Currency,
		&res.BillingCycle, &res.ExpiresAt, &res.PurchasedAt, &res.Specs, &tagsJSON,
		&res.Notes, &res.LastConnectedAt, &res.CreatedAt, &res.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return res, ErrNotFound
	}
	if err != nil {
		return res, err
	}
	res.HasPassword = pwdEnc != ""
	res.HasPrivateKey = keyEnc != "" || res.KeyID != ""
	if err := json.Unmarshal([]byte(tagsJSON), &res.Tags); err != nil || res.Tags == nil {
		res.Tags = []string{}
	}
	if err := json.Unmarshal([]byte(endpointsJSON), &res.Endpoints); err != nil || res.Endpoints == nil {
		res.Endpoints = []HostEndpoint{}
	}
	if res.Password, err = s.cipher.Decrypt(pwdEnc); err != nil {
		return res, err
	}
	if res.PrivateKey, err = s.cipher.Decrypt(keyEnc); err != nil {
		return res, err
	}
	if res.Passphrase, err = s.cipher.Decrypt(passEnc); err != nil {
		return res, err
	}
	if res.AuthType == "key" && res.KeyID != "" {
		if k, kErr := s.GetKeyWithSecret(res.KeyID); kErr == nil {
			res.PrivateKey = k.PrivateKey
			res.Passphrase = k.Passphrase
		}
	}
	return res, nil
}

// GetHostWithEndpoint returns a host with Host/Port overridden if endpointID matches one of its Endpoints.
func (s *Store) GetHostWithEndpoint(id, endpointID string) (HostWithSecrets, error) {
	res, err := s.GetHostWithSecrets(id)
	if err != nil || endpointID == "" {
		return res, err
	}
	for _, ep := range res.Endpoints {
		if ep.ID == endpointID {
			res.Host.Host = ep.Host
			res.Port = ep.Port
			break
		}
	}
	return res, nil
}

// CreateHost creates a new VPS host record.
func (s *Store) CreateHost(in HostInput) (Host, error) {
	normalizeInput(&in)
	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.NewString()

	pwdEnc, err := s.cipher.Encrypt(in.Password)
	if err != nil {
		return Host{}, err
	}
	keyEnc, err := s.cipher.Encrypt(in.PrivateKey)
	if err != nil {
		return Host{}, err
	}
	passEnc, err := s.cipher.Encrypt(in.Passphrase)
	if err != nil {
		return Host{}, err
	}
	tagsBytes, _ := json.Marshal(in.Tags)
	endpointsBytes, _ := json.Marshal(in.Endpoints)

	_, err = s.db.Exec(`
		INSERT INTO hosts (
			id, name, group_name, provider, region, host, port, endpoints_json, username, auth_type, key_id,
			password_enc, private_key_enc, passphrase_enc, cost, currency, billing_cycle,
			expires_at, purchased_at, specs, tags_json, notes, last_connected_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		id, in.Name, in.GroupName, in.Provider, in.Region, in.Host, in.Port, string(endpointsBytes),
		in.Username, in.AuthType, in.KeyID, pwdEnc, keyEnc, passEnc, in.Cost, in.Currency,
		in.BillingCycle, in.ExpiresAt, in.PurchasedAt, in.Specs, string(tagsBytes),
		in.Notes, now, now,
	)
	if err != nil {
		return Host{}, err
	}
	created, err := s.GetHostWithSecrets(id)
	return created.Host, err
}

// UpdateHost updates an existing host; empty secret fields preserve existing stored secrets.
func (s *Store) UpdateHost(id string, in HostInput) (Host, error) {
	existing, err := s.GetHostWithSecrets(id)
	if err != nil {
		return Host{}, err
	}
	normalizeInput(&in)
	now := time.Now().UTC().Format(time.RFC3339)

	pwdPlain := existing.Password
	if in.Password != "" {
		pwdPlain = in.Password
	}
	keyPlain := existing.PrivateKey
	passPlain := existing.Passphrase
	if in.AuthType == "key" && in.KeyID != "" {
		keyPlain = ""
		passPlain = ""
	} else {
		if in.PrivateKey != "" {
			keyPlain = in.PrivateKey
		}
		if in.Passphrase != "" {
			passPlain = in.Passphrase
		}
	}

	pwdEnc, err := s.cipher.Encrypt(pwdPlain)
	if err != nil {
		return Host{}, err
	}
	keyEnc, err := s.cipher.Encrypt(keyPlain)
	if err != nil {
		return Host{}, err
	}
	passEnc, err := s.cipher.Encrypt(passPlain)
	if err != nil {
		return Host{}, err
	}
	tagsBytes, _ := json.Marshal(in.Tags)
	endpointsBytes, _ := json.Marshal(in.Endpoints)

	_, err = s.db.Exec(`
		UPDATE hosts SET
			name = ?, group_name = ?, provider = ?, region = ?, host = ?, port = ?, endpoints_json = ?,
			username = ?, auth_type = ?, key_id = ?, password_enc = ?, private_key_enc = ?,
			passphrase_enc = ?, cost = ?, currency = ?, billing_cycle = ?,
			expires_at = ?, purchased_at = ?, specs = ?, tags_json = ?, notes = ?,
			updated_at = ?
		WHERE id = ?`,
		in.Name, in.GroupName, in.Provider, in.Region, in.Host, in.Port, string(endpointsBytes),
		in.Username, in.AuthType, in.KeyID, pwdEnc, keyEnc, passEnc, in.Cost, in.Currency,
		in.BillingCycle, in.ExpiresAt, in.PurchasedAt, in.Specs, string(tagsBytes),
		in.Notes, now, id,
	)
	if err != nil {
		return Host{}, err
	}
	updated, err := s.GetHostWithSecrets(id)
	return updated.Host, err
}

// DeleteHost deletes a host by ID.
func (s *Store) DeleteHost(id string) error {
	res, err := s.db.Exec(`DELETE FROM hosts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchLastConnected records the current timestamp as last_connected_at.
func (s *Store) TouchLastConnected(id string) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(`UPDATE hosts SET last_connected_at = ? WHERE id = ?`, now, id)
}


// SSHKey represents a stored SSH keypair (private key redacted).
type SSHKey struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Algorithm     string `json:"algorithm"`
	PublicKey     string `json:"publicKey"`
	Fingerprint   string `json:"fingerprint"`
	Comment       string `json:"comment"`
	HasPassphrase bool   `json:"hasPassphrase"`
	CreatedAt     string `json:"createdAt"`
}

// SSHKeyWithSecret includes the decrypted private key and passphrase.
type SSHKeyWithSecret struct {
	SSHKey
	PrivateKey string `json:"privateKey"`
	Passphrase string `json:"passphrase"`
}

// ListKeys returns all stored SSH keys without exposing private keys.
func (s *Store) ListKeys() ([]SSHKey, error) {
	rows, err := s.db.Query(`
		SELECT id, name, algorithm, public_key, passphrase_enc, fingerprint, comment, created_at
		FROM ssh_keys
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := []SSHKey{}
	for rows.Next() {
		var k SSHKey
		var passEnc string
		if err := rows.Scan(
			&k.ID, &k.Name, &k.Algorithm, &k.PublicKey, &passEnc,
			&k.Fingerprint, &k.Comment, &k.CreatedAt,
		); err != nil {
			return nil, err
		}
		k.HasPassphrase = passEnc != ""
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// GetKeyWithSecret returns a stored SSH key along with its decrypted private key.
func (s *Store) GetKeyWithSecret(id string) (SSHKeyWithSecret, error) {
	var res SSHKeyWithSecret
	var privEnc, passEnc string
	err := s.db.QueryRow(`
		SELECT id, name, algorithm, public_key, private_key_enc, passphrase_enc, fingerprint, comment, created_at
		FROM ssh_keys WHERE id = ?`, id).Scan(
		&res.ID, &res.Name, &res.Algorithm, &res.PublicKey, &privEnc, &passEnc,
		&res.Fingerprint, &res.Comment, &res.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return res, ErrNotFound
	}
	if err != nil {
		return res, err
	}
	res.HasPassphrase = passEnc != ""
	if res.PrivateKey, err = s.cipher.Decrypt(privEnc); err != nil {
		return res, err
	}
	if res.Passphrase, err = s.cipher.Decrypt(passEnc); err != nil {
		return res, err
	}
	return res, nil
}

// CreateKey stores a new SSH keypair with encrypted private key.
func (s *Store) CreateKey(name, algorithm, publicKey, privateKey, passphrase, fingerprint, comment string) (SSHKey, error) {
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)

	privEnc, err := s.cipher.Encrypt(privateKey)
	if err != nil {
		return SSHKey{}, err
	}
	passEnc, err := s.cipher.Encrypt(passphrase)
	if err != nil {
		return SSHKey{}, err
	}

	_, err = s.db.Exec(`
		INSERT INTO ssh_keys (id, name, algorithm, public_key, private_key_enc, passphrase_enc, fingerprint, comment, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, strings.TrimSpace(name), algorithm, publicKey, privEnc, passEnc, fingerprint, strings.TrimSpace(comment), now,
	)
	if err != nil {
		return SSHKey{}, err
	}
	created, err := s.GetKeyWithSecret(id)
	return created.SSHKey, err
}

// UpdateKey updates an existing SSH key's metadata or key material.
func (s *Store) UpdateKey(id, name, algorithm, publicKey, privateKey, passphrase, fingerprint, comment string) (SSHKey, error) {
	existing, err := s.GetKeyWithSecret(id)
	if err != nil {
		return SSHKey{}, err
	}
	if strings.TrimSpace(name) != "" {
		existing.Name = strings.TrimSpace(name)
	}
	if strings.TrimSpace(algorithm) != "" {
		existing.Algorithm = strings.TrimSpace(algorithm)
	}
	if strings.TrimSpace(publicKey) != "" {
		existing.PublicKey = strings.TrimSpace(publicKey)
	}
	if strings.TrimSpace(fingerprint) != "" {
		existing.Fingerprint = strings.TrimSpace(fingerprint)
	}
	existing.Comment = strings.TrimSpace(comment)

	privPlain := existing.PrivateKey
	if strings.TrimSpace(privateKey) != "" {
		privPlain = privateKey
	}
	passPlain := existing.Passphrase
	if passphrase != "" {
		passPlain = passphrase
	}

	privEnc, err := s.cipher.Encrypt(privPlain)
	if err != nil {
		return SSHKey{}, err
	}
	passEnc, err := s.cipher.Encrypt(passPlain)
	if err != nil {
		return SSHKey{}, err
	}

	_, err = s.db.Exec(`
		UPDATE ssh_keys
		SET name = ?, algorithm = ?, public_key = ?, private_key_enc = ?, passphrase_enc = ?, fingerprint = ?, comment = ?
		WHERE id = ?`,
		existing.Name, existing.Algorithm, existing.PublicKey, privEnc, passEnc, existing.Fingerprint, existing.Comment, id,
	)
	if err != nil {
		return SSHKey{}, err
	}
	updated, err := s.GetKeyWithSecret(id)
	return updated.SSHKey, err
}

// DeleteKey removes a stored SSH key and detaches it from any hosts referencing it.
func (s *Store) DeleteKey(id string) error {
	res, err := s.db.Exec(`DELETE FROM ssh_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	_, _ = s.db.Exec(`UPDATE hosts SET key_id = '' WHERE key_id = ?`, id)
	return nil
}

// SetHostKeyAuth switches a host's active authentication method to a Key Vault key ID.
func (s *Store) SetHostKeyAuth(hostID, keyID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE hosts SET auth_type = 'key', key_id = ?, updated_at = ? WHERE id = ?`, keyID, now, hostID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Invoice represents an uploaded billing invoice/receipt attached to a VPS host.
type Invoice struct {
	ID          string  `json:"id"`
	HostID      string  `json:"hostId"`
	Title       string  `json:"title"`
	Filename    string  `json:"filename"`
	StoragePath string  `json:"-"`
	MimeType    string  `json:"mimeType"`
	Size        int64   `json:"size"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	BilledAt    string  `json:"billedAt"`
	Notes       string  `json:"notes"`
	CreatedAt   string  `json:"createdAt"`
}

// ListInvoices returns all invoices for a host (or all hosts if hostID is empty).
func (s *Store) ListInvoices(hostID string) ([]Invoice, error) {
	var rows *sql.Rows
	var err error
	if hostID != "" {
		rows, err = s.db.Query(`
			SELECT id, host_id, title, filename, storage_path, mime_type, size, amount, currency, billed_at, notes, created_at
			FROM invoices WHERE host_id = ?
			ORDER BY billed_at DESC, created_at DESC`, hostID)
	} else {
		rows, err = s.db.Query(`
			SELECT id, host_id, title, filename, storage_path, mime_type, size, amount, currency, billed_at, notes, created_at
			FROM invoices
			ORDER BY billed_at DESC, created_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Invoice{}
	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(
			&inv.ID, &inv.HostID, &inv.Title, &inv.Filename, &inv.StoragePath,
			&inv.MimeType, &inv.Size, &inv.Amount, &inv.Currency, &inv.BilledAt,
			&inv.Notes, &inv.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, inv)
	}
	return list, rows.Err()
}

// GetInvoice returns a single invoice by ID.
func (s *Store) GetInvoice(id string) (Invoice, error) {
	var inv Invoice
	err := s.db.QueryRow(`
		SELECT id, host_id, title, filename, storage_path, mime_type, size, amount, currency, billed_at, notes, created_at
		FROM invoices WHERE id = ?`, id).Scan(
		&inv.ID, &inv.HostID, &inv.Title, &inv.Filename, &inv.StoragePath,
		&inv.MimeType, &inv.Size, &inv.Amount, &inv.Currency, &inv.BilledAt,
		&inv.Notes, &inv.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return inv, ErrNotFound
	}
	return inv, err
}

// CreateInvoice inserts a new invoice record.
func (s *Store) CreateInvoice(inv Invoice) (Invoice, error) {
	if inv.ID == "" {
		inv.ID = uuid.NewString()
	}
	if inv.CreatedAt == "" {
		inv.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if inv.Currency == "" {
		inv.Currency = "USD"
	}
	if strings.TrimSpace(inv.Title) == "" {
		inv.Title = inv.Filename
	}

	_, err := s.db.Exec(`
		INSERT INTO invoices (id, host_id, title, filename, storage_path, mime_type, size, amount, currency, billed_at, notes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.HostID, strings.TrimSpace(inv.Title), inv.Filename, inv.StoragePath,
		inv.MimeType, inv.Size, inv.Amount, inv.Currency, inv.BilledAt, strings.TrimSpace(inv.Notes), inv.CreatedAt,
	)
	if err != nil {
		return Invoice{}, err
	}
	return inv, nil
}

// DeleteInvoice removes an invoice record and returns the removed Invoice (so caller can delete the file).
func (s *Store) DeleteInvoice(id string) (Invoice, error) {
	inv, err := s.GetInvoice(id)
	if err != nil {
		return Invoice{}, err
	}
	if _, err := s.db.Exec(`DELETE FROM invoices WHERE id = ?`, id); err != nil {
		return Invoice{}, err
	}
	return inv, nil
}

// GetSetting retrieves and decrypts an encrypted setting value by key.
func (s *Store) GetSetting(key string) (string, error) {
	var enc string
	err := s.db.QueryRow(`SELECT value_enc FROM settings WHERE key = ?`, key).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return s.cipher.Decrypt(enc)
}

// SetSetting encrypts and stores a setting value by key.
func (s *Store) SetSetting(key, val string) error {
	enc, err := s.cipher.Encrypt(val)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO settings (key, value_enc) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value_enc = excluded.value_enc`,
		key, enc,
	)
	return err
}

// UpsertHostEndpoint adds or updates a labeled endpoint (such as "Tailscale") on a host.
func (s *Store) UpsertHostEndpoint(hostID string, ep HostEndpoint) (Host, error) {
	h, err := s.GetHostWithSecrets(hostID)
	if err != nil {
		return Host{}, err
	}
	if ep.ID == "" {
		ep.ID = uuid.NewString()
	}
	if ep.Port <= 0 {
		ep.Port = 22
	}

	replaced := false
	for i, existing := range h.Endpoints {
		if strings.EqualFold(existing.Label, ep.Label) && ep.Label != "" {
			ep.ID = existing.ID
			h.Endpoints[i] = ep
			replaced = true
			break
		}
	}
	if !replaced {
		h.Endpoints = append(h.Endpoints, ep)
	}

	b, _ := json.Marshal(h.Endpoints)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE hosts SET endpoints_json = ?, updated_at = ? WHERE id = ?`, string(b), now, hostID); err != nil {
		return Host{}, err
	}
	updated, err := s.GetHostWithSecrets(hostID)
	return updated.Host, err
}

// ReadMasterMeta reads master_salt and master_verifier from vpscoop.db without needing an unlocked Cipher.
func ReadMasterMeta(dbPath string) (salt string, verifier string, err error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", "", err
	}
	defer db.Close()

	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	_ = db.QueryRow(`SELECT value FROM meta WHERE key = 'master_salt'`).Scan(&salt)
	_ = db.QueryRow(`SELECT value FROM meta WHERE key = 'master_verifier'`).Scan(&verifier)
	return salt, verifier, nil
}

// WriteMasterMeta stores master_salt and master_verifier inside vpscoop.db.
func (s *Store) WriteMasterMeta(salt, verifier string) error {
	_, err := s.db.Exec(`
		INSERT INTO meta (key, value) VALUES ('master_salt', ?), ('master_verifier', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		salt, verifier,
	)
	return err
}
