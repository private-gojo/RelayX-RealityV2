package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Config holds all configuration for the relay server/client.
type Config struct {
	Mode          string
	Port          string
	XrayAddr      string
	XrayWSAddr    string
	XrayBin       string
	WSPath        string
	ChainPath     string
	XHTTPPath     string
	LocalAddr     string
	CloudRunHost  string
	RelayToken    string
	UUID          string
	PrivateKey    string
	PublicKey     string
	ShortID       string
	RealityDest   string
	ServerNames   []string
	CloudRunWSURL string
}

const (
	// Verified Curve25519 keypair generated via `xray x25519`
	DefaultBakedUUID       = "47be59a9-103b-4065-9c3a-ebb9ecf8ffd3"
	DefaultBakedPrivateKey = "2I5TZL08U6Jt930W-VswdvrUqlWfDfpa52EEB8i1QXA"
	DefaultBakedPublicKey  = "xsGaX-SwwQLt9kan7DY732udvFJdpJMujPGkLfvozwM"
	DefaultBakedShortID    = "3e08bf958266d858"
	DefaultBakedRelayToken = "relayx-secret-2026"
	DefaultRealityDest     = "firebaseremoteconfigrealtime.googleapis.com:443"
	DefaultServerNamesCSV  = "googleapis.com,firebaseremoteconfigrealtime.googleapis.com"
	DefaultXrayWSAddr      = "127.0.0.1:18080"
	DefaultChainPath       = "/_chain"
	LegacyWSPathAlias      = "/_ws"
)

// LoadFromEnv loads configuration from environment variables with sensible defaults.
func LoadFromEnv() (*Config, error) {
	mode := getEnv("MODE", "server")
	if mode != "server" && mode != "client" {
		return nil, fmt.Errorf("invalid MODE %q: must be 'server' or 'client'", mode)
	}

	port := getEnv("PORT", "8080")
	xrayAddr := getEnv("XRAY_ADDR", "127.0.0.1:18443")
	xrayWSAddr := getEnv("XRAY_WS_ADDR", DefaultXrayWSAddr)
	xrayBin := getEnv("XRAY_BIN", "/usr/local/bin/xray")
	wsPath := getEnv("WS_PATH", "/reality")
	chainPath := getEnv("CHAIN_PATH", DefaultChainPath)
	xhttpPath := getEnv("XHTTP_PATH", "/xh")
	localAddr := getEnv("LOCAL_ADDR", "127.0.0.1:10443")
	cloudRunHost := getEnv("CLOUD_RUN_HOST", "")
	relayToken := getEnv("RELAY_TOKEN", DefaultBakedRelayToken)
	uuid := getEnv("VLESS_UUID", DefaultBakedUUID)
	privKey := getEnv("REALITY_PRIVATE_KEY", DefaultBakedPrivateKey)
	pubKey := getEnv("REALITY_PUBLIC_KEY", DefaultBakedPublicKey)
	shortID := getEnv("REALITY_SHORT_ID", DefaultBakedShortID)
	realityDest := getEnv("REALITY_DEST", DefaultRealityDest)
	serverNamesStr := getEnv("REALITY_SERVER_NAMES", DefaultServerNamesCSV)

	var serverNames []string
	for _, s := range strings.Split(serverNamesStr, ",") {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			serverNames = append(serverNames, trimmed)
		}
	}
	if len(serverNames) == 0 {
		serverNames = []string{"googleapis.com", "firebaseremoteconfigrealtime.googleapis.com"}
	}

	cfg := &Config{
		Mode:         mode,
		Port:         port,
		XrayAddr:     xrayAddr,
		XrayWSAddr:   xrayWSAddr,
		XrayBin:      xrayBin,
		WSPath:       wsPath,
		ChainPath:    chainPath,
		XHTTPPath:    xhttpPath,
		LocalAddr:    localAddr,
		CloudRunHost: cloudRunHost,
		RelayToken:   relayToken,
		UUID:         uuid,
		PrivateKey:   privKey,
		PublicKey:    pubKey,
		ShortID:      shortID,
		RealityDest:  realityDest,
		ServerNames:  serverNames,
	}

	if mode == "client" {
		if cfg.CloudRunHost == "" {
			return nil, errors.New("CLOUD_RUN_HOST is required in client mode")
		}
		scheme := "wss"
		if strings.HasPrefix(cfg.CloudRunHost, "127.0.0.1:") || strings.HasPrefix(cfg.CloudRunHost, "localhost:") {
			scheme = "ws"
		}
		cfg.CloudRunWSURL = fmt.Sprintf("%s://%s%s", scheme, cfg.CloudRunHost, cfg.WSPath)
	}

	return cfg, nil
}

// EnsureCredentials generates missing UUID, x25519 keys, and shortId if not provided.
func (c *Config) EnsureCredentials() error {
	if c.UUID == "" {
		u, err := generateUUID()
		if err != nil {
			return fmt.Errorf("failed to generate UUID: %w", err)
		}
		c.UUID = u
	}

	if c.PrivateKey == "" {
		priv, pub, err := generateX25519Keys(c.XrayBin)
		if err != nil {
			return fmt.Errorf("failed to generate x25519 keys: %w", err)
		}
		c.PrivateKey = priv
		c.PublicKey = pub
	}

	if c.ShortID == "" {
		sid, err := generateRandomHex(8)
		if err != nil {
			return fmt.Errorf("failed to generate shortId: %w", err)
		}
		c.ShortID = sid
	}

	return nil
}

// WithCloudRunHost returns a shallow copy of Config with CloudRunHost overridden.
func (c *Config) WithCloudRunHost(host string) *Config {
	cp := *c
	cp.CloudRunHost = strings.TrimSpace(host)
	return &cp
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func generateRandomHex(numBytes int) (string, error) {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generateX25519Keys(xrayBin string) (privateKey, publicKey string, err error) {
	cmd := exec.Command(xrayBin, "x25519")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("xray x25519 failed: %w (output: %s)", err, string(out))
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		lower := strings.ToLower(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		val := strings.TrimSpace(parts[1])
		if strings.Contains(lower, "private") {
			privateKey = val
		} else if strings.Contains(lower, "public") || strings.Contains(lower, "password") {
			publicKey = val
		}
	}

	if privateKey == "" {
		return "", "", fmt.Errorf("could not parse private key from xray output: %s", string(out))
	}
	return privateKey, publicKey, nil
}
