package keys

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// GeneratedKey holds a newly generated or parsed SSH keypair.
type GeneratedKey struct {
	Algorithm   string
	PublicKey   string
	PrivateKey  string
	Fingerprint string
}

// Generate creates a new SSH keypair in OpenSSH format.
// Supported algorithms: ed25519, ecdsa-p256, ecdsa-p384, ecdsa-p521, rsa-2048, rsa-3072, rsa-4096.
func Generate(algorithm, comment, passphrase string) (GeneratedKey, error) {
	alg := strings.ToLower(strings.TrimSpace(algorithm))
	if alg == "" {
		alg = "ed25519"
	}

	var rawPriv any
	var sshPub ssh.PublicKey

	switch alg {
	case "ed25519":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return GeneratedKey{}, fmt.Errorf("generate ed25519: %w", err)
		}
		rawPriv = priv
		sshPub, err = ssh.NewPublicKey(pub)
		if err != nil {
			return GeneratedKey{}, err
		}
	case "ecdsa", "ecdsa-p256":
		alg = "ecdsa-p256"
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return GeneratedKey{}, fmt.Errorf("generate ecdsa-p256: %w", err)
		}
		rawPriv = priv
		sshPub, err = ssh.NewPublicKey(&priv.PublicKey)
		if err != nil {
			return GeneratedKey{}, err
		}
	case "ecdsa-p384":
		priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			return GeneratedKey{}, fmt.Errorf("generate ecdsa-p384: %w", err)
		}
		rawPriv = priv
		sshPub, err = ssh.NewPublicKey(&priv.PublicKey)
		if err != nil {
			return GeneratedKey{}, err
		}
	case "ecdsa-p521":
		priv, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
		if err != nil {
			return GeneratedKey{}, fmt.Errorf("generate ecdsa-p521: %w", err)
		}
		rawPriv = priv
		sshPub, err = ssh.NewPublicKey(&priv.PublicKey)
		if err != nil {
			return GeneratedKey{}, err
		}
	case "rsa-2048":
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return GeneratedKey{}, fmt.Errorf("generate rsa-2048: %w", err)
		}
		rawPriv = priv
		sshPub, err = ssh.NewPublicKey(&priv.PublicKey)
		if err != nil {
			return GeneratedKey{}, err
		}
	case "rsa-3072":
		priv, err := rsa.GenerateKey(rand.Reader, 3072)
		if err != nil {
			return GeneratedKey{}, fmt.Errorf("generate rsa-3072: %w", err)
		}
		rawPriv = priv
		sshPub, err = ssh.NewPublicKey(&priv.PublicKey)
		if err != nil {
			return GeneratedKey{}, err
		}
	case "rsa", "rsa-4096":
		alg = "rsa-4096"
		priv, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return GeneratedKey{}, fmt.Errorf("generate rsa-4096: %w", err)
		}
		rawPriv = priv
		sshPub, err = ssh.NewPublicKey(&priv.PublicKey)
		if err != nil {
			return GeneratedKey{}, err
		}
	default:
		return GeneratedKey{}, fmt.Errorf("unsupported algorithm %q", algorithm)
	}

	var pemBlock *pem.Block
	var err error
	if passphrase != "" {
		pemBlock, err = ssh.MarshalPrivateKeyWithPassphrase(rawPriv, comment, []byte(passphrase))
	} else {
		pemBlock, err = ssh.MarshalPrivateKey(rawPriv, comment)
	}
	if err != nil {
		return GeneratedKey{}, fmt.Errorf("marshal private key: %w", err)
	}

	pubLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	if strings.TrimSpace(comment) != "" {
		pubLine = pubLine + " " + strings.TrimSpace(comment)
	}

	return GeneratedKey{
		Algorithm:   alg,
		PublicKey:   pubLine,
		PrivateKey:  string(pem.EncodeToMemory(pemBlock)),
		Fingerprint: ssh.FingerprintSHA256(sshPub),
	}, nil
}

// ParseExisting validates a PEM/OpenSSH private key and derives its public key, algorithm, and fingerprint.
func ParseExisting(privateKey, passphrase, comment string) (GeneratedKey, error) {
	trimmed := strings.TrimSpace(privateKey)
	if trimmed == "" {
		return GeneratedKey{}, errors.New("private key is required")
	}

	var signer ssh.Signer
	var err error
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(trimmed), []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey([]byte(trimmed))
	}
	if err != nil {
		return GeneratedKey{}, fmt.Errorf("invalid private key: %w", err)
	}

	sshPub := signer.PublicKey()
	alg := normalizePublicKeyType(sshPub.Type())
	pubLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	if strings.TrimSpace(comment) != "" {
		pubLine = pubLine + " " + strings.TrimSpace(comment)
	}

	return GeneratedKey{
		Algorithm:   alg,
		PublicKey:   pubLine,
		PrivateKey:  trimmed + "\n",
		Fingerprint: ssh.FingerprintSHA256(sshPub),
	}, nil
}

func normalizePublicKeyType(keyType string) string {
	switch keyType {
	case "ssh-ed25519":
		return "ed25519"
	case "ecdsa-sha2-nistp256":
		return "ecdsa-p256"
	case "ecdsa-sha2-nistp384":
		return "ecdsa-p384"
	case "ecdsa-sha2-nistp521":
		return "ecdsa-p521"
	case "ssh-rsa":
		return "rsa"
	default:
		return strings.TrimPrefix(keyType, "ssh-")
	}
}
