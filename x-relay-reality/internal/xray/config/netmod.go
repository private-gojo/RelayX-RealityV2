package xrayconfig

import (
	"encoding/json"
	"errors"

	appconfig "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/config"
)

// GenerateNetModProfileJSON returns a complete standalone Xray/NetMod JSON profile.
// Note: NetMod Syna binds its own local SOCKS port (10809) based on the first inbound entry.
func GenerateNetModProfileJSON(c *appconfig.Config) ([]byte, error) {
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
		chainPath = appconfig.DefaultChainPath
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
				"port":     10809,
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
						"mode": "stream-one",
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
					"ip":          []string{"0.0.0.0/8", "10.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16", "::/128", "::1/128", "fc00::/7", "fe80::/10"},
					"outboundTag": "direct",
				},
			},
		},
	}

	return json.MarshalIndent(profile, "", "  ")
}

