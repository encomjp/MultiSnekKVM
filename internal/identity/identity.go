package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DeviceInfo describes this device's identity for discovery and transport.
type DeviceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	PairingCode string `json:"pairingCode,omitempty"`
	Port        int    `json:"port"`
}

// Identity holds the loaded device identity.
type Identity struct {
	DeviceID    string
	Name        string
	Fingerprint string
}

type savedIdentity struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
}

// CertPath returns the path to the device TLS certificate.
func CertPath() string {
	dir, _ := DataDir()
	return filepath.Join(dir, "device-cert.pem")
}

// KeyPath returns the path to the device TLS private key.
func KeyPath() string {
	dir, _ := DataDir()
	return filepath.Join(dir, "device-key.pem")
}

// DataDir returns (and creates) the identity data directory.
func DataDir() (string, error) {
	root, err := AppDataDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "identity")
	return path, os.MkdirAll(path, 0o700)
}

// AppDataDir returns (and creates) the application config directory.
func AppDataDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "MultiSnekKVM")
	return path, os.MkdirAll(path, 0o700)
}

func LoadOrCreateIdentity(name string) (*Identity, error) {
	dir, err := DataDir()
	if err != nil {
		return nil, err
	}

	metaPath := filepath.Join(dir, "identity.json")
	certPath := filepath.Join(dir, "device-cert.pem")
	keyPath := filepath.Join(dir, "device-key.pem")

	var meta savedIdentity
	data, err := os.ReadFile(metaPath)
	if err == nil {
		_ = json.Unmarshal(data, &meta)
	}

	if meta.DeviceID == "" {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("generate device id: %w", err)
		}
		meta.DeviceID = hex.EncodeToString(buf)
		meta.Name = name
		data, _ = json.MarshalIndent(meta, "", "  ")
		if err := os.WriteFile(metaPath, data, 0o600); err != nil {
			return nil, fmt.Errorf("save identity: %w", err)
		}
	} else if name != "" && meta.Name != name {
		log.Printf("identity: hostname changed %q → %q, regenerating certificate", meta.Name, name)
		meta.Name = name
		data, _ = json.MarshalIndent(meta, "", "  ")
		_ = os.WriteFile(metaPath, data, 0o600)
		_ = os.Remove(certPath)
	}

	fingerprint, err := ensureCertificate(certPath, keyPath, meta)
	if err != nil {
		return nil, err
	}

	return &Identity{
		DeviceID:    meta.DeviceID,
		Name:        meta.Name,
		Fingerprint: fingerprint,
	}, nil
}

func ensureCertificate(certPath, keyPath string, meta savedIdentity) (string, error) {
	if certData, err := os.ReadFile(certPath); err == nil {
		block, _ := pem.Decode(certData)
		if block != nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil && certificateMatchesIdentity(cert, meta) {
				return CertificateFingerprint(cert), nil
			}
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", fmt.Errorf("generate certificate serial: %w", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   meta.Name,
			Organization: []string{"MultiSnekKVM"},
			SerialNumber: meta.DeviceID,
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		return "", fmt.Errorf("write certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "", fmt.Errorf("write private key: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return "", fmt.Errorf("parse generated certificate: %w", err)
	}
	return CertificateFingerprint(cert), nil
}

func certificateMatchesIdentity(cert *x509.Certificate, meta savedIdentity) bool {
	if cert == nil {
		return false
	}
	if strings.TrimSpace(cert.Subject.CommonName) != strings.TrimSpace(meta.Name) {
		return false
	}
	if strings.TrimSpace(cert.Subject.SerialNumber) != strings.TrimSpace(meta.DeviceID) {
		return false
	}
	if !ContainsStringFold(cert.Subject.Organization, "MultiSnekKVM") {
		return false
	}
	return hasExtKeyUsage(cert, x509.ExtKeyUsageClientAuth) && hasExtKeyUsage(cert, x509.ExtKeyUsageServerAuth)
}

func hasExtKeyUsage(cert *x509.Certificate, usage x509.ExtKeyUsage) bool {
	for _, candidate := range cert.ExtKeyUsage {
		if candidate == usage {
			return true
		}
	}
	return false
}

// ContainsStringFold reports whether any element of values equals candidate
// under Unicode case-folding.
func ContainsStringFold(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

// CertificateFingerprint returns the hex-encoded SHA-256 fingerprint of a certificate.
func CertificateFingerprint(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	fingerprint := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(fingerprint[:])
}
