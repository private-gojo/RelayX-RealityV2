package config

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	FixedUUID        = "5f8fe3bc-169b-4c8c-b4ba-5fc94abd6ce0"
	FixedPrivateKey  = "MD7Faoxwbj5bFx7vDdDpYaAvePia1H6CZ4CvHmrFemU"
	FixedPublicKey   = "xaTNSdK2aQv7ND8DxZcej-dtiEldJZUIRCoQHXkPwFw"
	FixedShortID     = "b25078a30229392b"
	FixedBridgeToken = "d0b392d0ce0f05a82dcee11d421c9b45da9790692c095510e8f7966"
	FixedServerName  = "firebaseremoteconfigrealtime.googleapis.com"
	FixedTarget      = "firebaseremoteconfigrealtime.googleapis.com:443"
	FixedCloudRunHost = "relayx-reality-v2-667648148327.asia-east1.run.app"
	FixedPath         = "/reality"
	ChainPath         = "/_chain"
	LegacyPath        = "/_ws"
	ChainInboundPath  = "/chain"
	ChainInboundPort  = 18080
	RealityInboundPort = 18443
)

var uuidPattern = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$")

type Config struct {
	UUID        string
	PrivateKey  string
	ShortID     string
	ServerName  string
	Target      string
	Path        string
	BridgeToken string
	Port        string
	MaxSessions int
}

func Load() (Config, error) {
	maxSessions := 256
	if value := os.Getenv("MAX_SESSIONS"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 1000 {
			return Config{}, errors.New("MAX_SESSIONS must be an integer from 1 to 1000")
		}
		maxSessions = n
	}

	cfg := Config{
		UUID:        FixedUUID,
		PrivateKey:  FixedPrivateKey,
		ShortID:     FixedShortID,
		ServerName:  FixedServerName,
		Target:      FixedTarget,
		Path:        FixedPath,
		BridgeToken: FixedBridgeToken,
		Port:        valueOr(os.Getenv("PORT"), "8080"),
		MaxSessions: maxSessions,
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadClient() (Config, error) {
	cfg, err := Load()
	if err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if !uuidPattern.MatchString(c.UUID) {
		return errors.New("VLESS_UUID must be a valid UUID")
	}

	privateBytes, err := base64.RawURLEncoding.DecodeString(c.PrivateKey)
	if err != nil || len(privateBytes) != 32 {
		return errors.New("REALITY_PRIVATE_KEY must be a 32-byte base64url X25519 private key without padding")
	}
	if _, err := ecdh.X25519().NewPrivateKey(privateBytes); err != nil {
		return fmt.Errorf("REALITY_PRIVATE_KEY is invalid: %w", err)
	}

	if len(c.ShortID) < 2 || len(c.ShortID) > 16 || len(c.ShortID)%2 != 0 {
		return errors.New("REALITY_SHORT_ID must contain 2 to 16 even-count hex characters")
	}
	if _, err := hex.DecodeString(c.ShortID); err != nil {
		return errors.New("REALITY_SHORT_ID must be hexadecimal")
	}
	if c.ServerName == "" || strings.ContainsAny(c.ServerName, " /\\\t\r\n") {
		return errors.New("REALITY_SERVER_NAME must be a hostname")
	}

	host, targetPort, err := net.SplitHostPort(c.Target)
	if err != nil || host == "" || targetPort != "443" {
		return errors.New("REALITY_TARGET must be a host on port 443, for example firebaseremoteconfigrealtime.googleapis.com:443")
	}
	if !strings.HasPrefix(c.Path, "/") || strings.ContainsAny(c.Path, "?# \t\r\n") {
		return errors.New("REALITY_PATH must start with / and must not contain a query, fragment, or whitespace")
	}
	if len(c.BridgeToken) < 32 {
		return errors.New("BRIDGE_TOKEN must be at least 32 characters")
	}
	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("PORT must be between 1 and 65535")
	}
	return nil
}

func (c Config) PublicKey() (string, error) {
	privateBytes, err := base64.RawURLEncoding.DecodeString(c.PrivateKey)
	if err != nil {
		return "", err
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		return "", err
	}
	derived := base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes())
	if derived != FixedPublicKey {
		return "", errors.New("embedded REALITY public/private key pair does not match")
	}
	return FixedPublicKey, nil
}

func (c Config) ServerJSON() ([]byte, error) {
	config := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []any{
			map[string]any{
				"tag":      "chain-ws",
				"listen":   "127.0.0.1",
				"port":     ChainInboundPort,
				"protocol": "vless",
				"settings": map[string]any{
					"clients":    []any{map[string]any{"id": c.UUID}},
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"method":   "websocket",
					"security": "none",
					"wsSettings": map[string]any{
						"path": ChainInboundPath,
					},
				},
			},
			map[string]any{
				"tag":      "reality-xhttp",
				"listen":   "127.0.0.1",
				"port":     RealityInboundPort,
				"protocol": "vless",
				"settings": map[string]any{
					"clients":    []any{map[string]any{"id": c.UUID}},
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"method":   "xhttp",
					"security": "reality",
					"realitySettings": map[string]any{
						"show":        false,
						"target":      c.Target,
						"xver":        0,
						"serverNames": []string{c.ServerName},
						"privateKey":  c.PrivateKey,
						"shortIds":    []string{c.ShortID},
					},
					"xhttpSettings": map[string]any{
						"path": c.Path,
						"mode": "auto",
					},
				},
			},
		},
		"outbounds": []any{
			map[string]any{
				"tag":      "direct",
				"protocol": "freedom",
			},
			map[string]any{
				"tag":      "chain-redirect",
				"protocol": "freedom",
				"settings": map[string]any{
					"redirect": "127.0.0.1:18443",
					"finalRules": []any{
						map[string]any{
							"action":  "allow",
							"network": "tcp",
							"ip":      []string{"127.0.0.1/32"},
							"port":    "18443",
						},
					},
				},
			},
		},
		"routing": map[string]any{
			"rules": []any{
				map[string]any{
					"type":        "field",
					"inboundTag":  []string{"chain-ws"},
					"network":     "tcp",
					"outboundTag": "chain-redirect",
				},
			},
		},
	}
	return json.MarshalIndent(config, "", "  ")
}

// ClientJSON preserves the optional Termux adapter profile.
func (c Config) ClientJSON() ([]byte, error) {
	publicKey, err := c.PublicKey()
	if err != nil {
		return nil, err
	}
	config := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []any{
			map[string]any{
				"listen":   "127.0.0.1",
				"port":     10808,
				"protocol": "socks",
				"settings": map[string]any{"udp": true},
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
							"port":    11000,
							"users": []any{
								map[string]any{"id": c.UUID, "encryption": "none"},
							},
						},
					},
				},
				"streamSettings": map[string]any{
					"method":   "xhttp",
					"security": "reality",
					"realitySettings": map[string]any{
						"serverName":  c.ServerName,
						"fingerprint": "chrome",
						"password":    publicKey,
						"shortId":     c.ShortID,
					},
					"xhttpSettings": map[string]any{
						"path": c.Path,
						"mode": "auto",
					},
				},
			},
		},
	}
	return json.MarshalIndent(config, "", "  ")
}

// NetModJSON returns a complete Xray config with the Cloud Run WS carrier as a dialerProxy.
func (c Config) NetModJSON(cloudRunHost string) ([]byte, error) {
	host, err := normalizeHost(cloudRunHost)
	if err != nil {
		return nil, err
	}
	publicKey, err := c.PublicKey()
	if err != nil {
		return nil, err
	}

	config := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []any{
			map[string]any{
				"listen":   "127.0.0.1",
				"port":     10808,
				"protocol": "socks",
				"settings": map[string]any{"udp": true},
			},
		},
		"outbounds": []any{
			map[string]any{
				"tag":      "proxy",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []any{
						map[string]any{
							"address": c.ServerName,
							"port":    443,
							"users": []any{
								map[string]any{"id": c.UUID, "encryption": "none"},
							},
						},
					},
				},
				"streamSettings": map[string]any{
					"method":   "xhttp",
					"security": "reality",
					"realitySettings": map[string]any{
						"serverName":  c.ServerName,
						"fingerprint": "chrome",
						"password":    publicKey,
						"shortId":     c.ShortID,
					},
					"xhttpSettings": map[string]any{
						"path": c.Path,
						"mode": "auto",
					},
					"sockopt": map[string]any{
						"dialerProxy": "cloudrun-chain",
					},
				},
			},
			map[string]any{
				"tag":      "cloudrun-chain",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []any{
						map[string]any{
							"address": host,
							"port":    443,
							"users": []any{
								map[string]any{"id": c.UUID, "encryption": "none"},
							},
						},
					},
				},
				"streamSettings": map[string]any{
					"method":   "websocket",
					"security": "tls",
					"tlsSettings": map[string]any{
						"serverName": host,
						"alpn":       []string{"http/1.1"},
					},
					"wsSettings": map[string]any{
						"path":            ChainPath,
						"host":            host,
						"heartbeatPeriod": 20,
						"headers": map[string]any{
							"Authorization": "Bearer " + c.BridgeToken,
						},
					},
				},
			},
		},
	}
	return json.MarshalIndent(config, "", "  ")
}

func normalizeHost(host string) (string, error) {
	host = strings.TrimSpace(strings.ToLower(host))
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "/\\?# \t\r\n") {
		return "", errors.New("Cloud Run host must be a hostname without a scheme, path, or query")
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return "", errors.New("Cloud Run host must be a fully qualified hostname")
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("Cloud Run host contains an invalid DNS label")
		}
		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
				return "", errors.New("Cloud Run host contains an invalid DNS character")
			}
		}
	}
	return host, nil
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
