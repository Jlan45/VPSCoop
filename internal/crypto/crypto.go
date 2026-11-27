package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/pbkdf2"
)

// Cipher provides AES-256-GCM encryption and decryption for stored credentials.
type Cipher struct {
	aead cipher.AEAD
}

// GenerateSalt returns 16 cryptographically random bytes.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// DeriveCipher derives a 256-bit AES-GCM Cipher from passphrase and salt using PBKDF2-SHA256.
func DeriveCipher(passphrase string, salt []byte) (*Cipher, error) {
	if len(passphrase) < 4 {
		return nil, errors.New("主密钥长度至少需要 4 个字符")
	}
	if len(salt) < 8 {
		return nil, errors.New("invalid salt")
	}
	key := pbkdf2.Key([]byte(passphrase), salt, 100000, 32, sha256.New)
	return newCipherFromKey(key)
}

// HasLegacyKey checks if VPSCOOP_SECRET_KEY env or a legacy .vpscoop.key file exists.
func HasLegacyKey(dataDir string) bool {
	if os.Getenv("VPSCOOP_SECRET_KEY") != "" {
		return true
	}
	keyPath := filepath.Join(dataDir, ".vpscoop.key")
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		return false
	}
	decoded, decErr := base64.StdEncoding.DecodeString(string(raw))
	return decErr == nil && len(decoded) == 32
}

// NewCipher initializes a Cipher from VPSCOOP_SECRET_KEY or a legacy .vpscoop.key file in dataDir.
func NewCipher(dataDir string) (*Cipher, error) {
	var key []byte

	if envKey := os.Getenv("VPSCOOP_SECRET_KEY"); envKey != "" {
		sum := sha256.Sum256([]byte(envKey))
		key = sum[:]
	} else {
		keyPath := filepath.Join(dataDir, ".vpscoop.key")
		raw, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, errors.New("master key not initialized")
		}
		decoded, decErr := base64.StdEncoding.DecodeString(string(raw))
		if decErr != nil || len(decoded) != 32 {
			return nil, errors.New("invalid master key file")
		}
		key = decoded
	}

	return newCipherFromKey(key)
}

func newCipherFromKey(key []byte) (*Cipher, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt encrypts plaintext into a base64-encoded nonce+ciphertext string.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decrypts a base64-encoded nonce+ciphertext string back into plaintext.
func (c *Cipher) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plain, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
