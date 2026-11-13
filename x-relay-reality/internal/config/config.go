package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	DefaultBakedUUID       = "1ef40695-9d5b-498e-aeaa-c44ab281ca61"
	DefaultBakedPrivateKey = "eKS-gh5AXjEdJ3sGF7hDpUhFlKGJ615192I4no0yFGo"
	DefaultBakedPublicKey  = "g5fbef1N1N6sR6eF1B8cR-gM7l5vE9m8pW5qX2zK1yA"
	DefaultBakedShortID    = "6ca30cd71da67803"
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

// GenerateServerJSON returns the Xray server configuration JSON bytes.
// It configures two localhost inbounds:
//  1. VLESS+WS on XrayWSAddr (default 127.0.0.1:18080) for the outer Cloud Run transport leg.
//  2. VLESS+XHTTP+REALITY on XrayAddr (default 127.0.0.1:18443) for the inner REALITY leg.
func (c *Config) GenerateServerJSON() ([]byte, error) {
	if c.UUID == "" || c.PrivateKey == "" {
		return nil, errors.New("UUID and PrivateKey must be set before generating server config")
	}

	host, portStr, err := splitHostPort(c.XrayAddr, "127.0.0.1", 18443)
	if err != nil {
		return nil, err
	}

	wsHost, wsPort, err := splitHostPort(c.XrayWSAddr, "127.0.0.1", 18080)
	if err != nil {
		return nil, err
	}

	chainPath := c.ChainPath
	if chainPath == "" {
		chainPath = DefaultChainPath
	}

	serverConfig := map[string]any{
		"log": map[string]any{
			"loglevel": "warning",
		},
		"inbounds": []any{
			map[string]any{
				"tag":      "outer-ws-in",
				"listen":   wsHost,
				"port":     wsPort,
				"protocol": "vless",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{
							"id":   c.UUID,
							"flow": "",
						},
					},
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"network":  "ws",
					"security": "none",
					"wsSettings": map[string]any{
						"path": chainPath,
					},
				},
			},
			map[string]any{
				"tag":      "reality-xhttp-in",
				"listen":   host,
				"port":     portStr,
				"protocol": "vless",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{
							"id":   c.UUID,
							"flow": "",
						},
					},
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"network":  "xhttp",
					"security": "reality",
					"xhttpSettings": map[string]any{
						"mode": "auto",
						"path": c.XHTTPPath,
					},
					"realitySettings": map[string]any{
						"show":        false,
						"dest":        c.RealityDest,
						"xver":        0,
						"serverNames": c.ServerNames,
						"privateKey":  c.PrivateKey,
						"shortIds":    []string{"", c.ShortID},
					},
				},
			},
		},
		"outbounds": []any{
			map[string]any{
				"tag":      "direct",
				"protocol": "freedom",
				"settings": map[string]any{},
			},
			map[string]any{
				"tag":      "loopback-to-reality",
				"protocol": "freedom",
				"settings": map[string]any{
					"redirect": fmt.Sprintf("%s:%d", host, portStr),
				},
			},
			map[string]any{
				"tag":      "block",
				"protocol": "blackhole",
				"settings": map[string]any{},
			},
		},
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules": []any{
				map[string]any{
					"type":        "field",
					"inboundTag":  []string{"outer-ws-in"},
					"outboundTag": "loopback-to-reality",
				},
				map[string]any{
					"type":        "field",
					"inboundTag":  []string{"reality-xhttp-in"},
					"outboundTag": "direct",
				},
			},
		},
	}

	return json.MarshalIndent(serverConfig, "", "  ")
}

// GenerateClientJSON returns a template Xray client configuration JSON for legacy local Termux adapter mode.
func (c *Config) GenerateClientJSON() ([]byte, error) {
	host, portNum, err := splitHostPort(c.LocalAddr, "127.0.0.1", 10443)
	if err != nil {
		return nil, err
	}

	sni := primarySNI(c.ServerNames)

	clientConfig := map[string]any{
		"log": map[string]any{
			"loglevel": "warning",
		},
		"inbounds": []any{
			map[string]any{
				"tag":      "socks-in",
				"port":     10808,
				"listen":   "127.0.0.1",
				"protocol": "socks",
				"settings": map[string]any{
					"auth": "noauth",
					"udp":  true,
				},
			},
		},
		"outbounds": []any{
			map[string]any{
				"tag":      "proxy",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []any{
						map[string]any{
							"address": host,
							"port":    portNum,
							"users": []any{
								map[string]any{
									"id":         c.UUID,
									"encryption": "none",
									"flow":       "",
								},
							},
						},
					},
				},
				"streamSettings": map[string]any{
					"network":  "xhttp",
					"security": "reality",
					"xhttpSettings": map[string]any{
						"mode": "auto",
						"path": c.XHTTPPath,
					},
					"realitySettings": map[string]any{
						"show":        false,
						"serverName":  sni,
						"fingerprint": "chrome",
						"publicKey":   c.PublicKey,
						"shortId":     c.ShortID,
						"spiderX":     "/",
					},
				},
			},
		},
	}

	return json.MarshalIndent(clientConfig, "", "  ")
}

// GenerateNetModProfileJSON returns a complete standalone Xray/NetMod JSON profile that uses
// sockopt.dialerProxy to chain VLESS+XHTTP+REALITY over an outer VLESS+WS+TLS Cloud Run leg
// without requiring Termux or any external local adapter.
func (c *Config) GenerateNetModProfileJSON() ([]byte, error) {
	if c.CloudRunHost == "" {
		return nil, errors.New("CloudRunHost is required to generate a NetMod profile")
	}
	if c.UUID == "" || c.PublicKey == "" {
		return nil, errors.New("UUID and PublicKey must be set before generating a NetMod profile")
	}

	_, realityPort, err := splitHostPort(c.XrayAddr, "127.0.0.1", 18443)
	if err != nil {
		return nil, err
	}

	sni := primarySNI(c.ServerNames)
	chainPath := c.ChainPath
	if chainPath == "" {
		chainPath = DefaultChainPath
	}

	wsHeaders := map[string]any{
		"Host": c.CloudRunHost,
	}
	if c.RelayToken != "" {
		wsHeaders["X-Relay-Auth"] = c.RelayToken
	}

	profile := map[string]any{
		"remarks": "RelayX-RealityV2 (Zero-Termux NetMod)",
		"log": map[string]any{
			"loglevel": "warning",
		},
		"inbounds": []any{
			map[string]any{
				"tag":      "socks-in",
				"listen":   "127.0.0.1",
				"port":     10808,
				"protocol": "socks",
				"settings": map[string]any{
					"auth": "noauth",
					"udp":  true,
				},
				"sniffing": map[string]any{
					"enabled":      true,
					"destOverride": []string{"http", "tls"},
				},
			},
			map[string]any{
				"tag":      "http-in",
				"listen":   "127.0.0.1",
				"port":     10809,
				"protocol": "http",
				"sniffing": map[string]any{
					"enabled":      true,
					"destOverride": []string{"http", "tls"},
				},
			},
		},
		"outbounds": []any{
			map[string]any{
				"tag":      "proxy",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []any{
						map[string]any{
							"address": "127.0.0.1",
							"port":    realityPort,
							"users": []any{
								map[string]any{
									"id":         c.UUID,
									"encryption": "none",
									"flow":       "",
								},
							},
						},
					},
				},
				"streamSettings": map[string]any{
					"network":  "xhttp",
					"security": "reality",
					"xhttpSettings": map[string]any{
						"mode": "auto",
						"path": c.XHTTPPath,
					},
					"realitySettings": map[string]any{
						"show":        false,
						"serverName":  sni,
						"fingerprint": "chrome",
						"publicKey":   c.PublicKey,
						"shortId":     c.ShortID,
						"spiderX":     "/",
					},
					"sockopt": map[string]any{
						"dialerProxy": "outer-ws",
					},
				},
			},
			map[string]any{
				"tag":      "outer-ws",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []any{
						map[string]any{
							"address": sni,
							"port":    443,
							"users": []any{
								map[string]any{
									"id":         c.UUID,
									"encryption": "none",
									"flow":       "",
								},
							},
						},
					},
				},
				"streamSettings": map[string]any{
					"network":  "ws",
					"security": "tls",
					"tlsSettings": map[string]any{
						"serverName":  sni,
						"fingerprint": "chrome",
						"alpn":        []string{"http/1.1"},
					},
					"wsSettings": map[string]any{
						"path":    chainPath,
						"headers": wsHeaders,
					},
				},
			},
			map[string]any{
				"tag":      "direct",
				"protocol": "freedom",
				"settings": map[string]any{},
			},
			map[string]any{
				"tag":      "block",
				"protocol": "blackhole",
				"settings": map[string]any{},
			},
		},
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules": []any{
				map[string]any{
					"type":        "field",
					"ip": []string{
						"0.0.0.0/8",
						"10.0.0.0/8",
						"127.0.0.0/8",
						"169.254.0.0/16",
						"172.16.0.0/12",
						"192.168.0.0/16",
						"::/128",
						"::1/128",
						"fc00::/7",
						"fe80::/10",
					},
					"outboundTag": "direct",
				},
			},
		},
	}

	return json.MarshalIndent(profile, "", "  ")
}

// WithCloudRunHost returns a shallow copy of Config with CloudRunHost overridden.
func (c *Config) WithCloudRunHost(host string) *Config {
	cp := *c
	cp.CloudRunHost = strings.TrimSpace(host)
	return &cp
}

func primarySNI(serverNames []string) string {
	sni := "firebaseremoteconfigrealtime.googleapis.com"
	for _, name := range serverNames {
		if strings.Contains(name, "firebaseremoteconfigrealtime") {
			return name
		}
	}
	if len(serverNames) > 0 {
		return serverNames[0]
	}
	return sni
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func splitHostPort(addr, defaultHost string, defaultPort int) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return defaultHost, defaultPort, nil
	}
	var port int
	_, err := fmt.Sscanf(parts[1], "%d", &port)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port in %q: %w", addr, err)
	}
	return parts[0], port, nil
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
