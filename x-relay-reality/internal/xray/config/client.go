package xrayconfig

import (
	"encoding/json"

	appconfig "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/config"
)

// GenerateClientJSON returns a template Xray client configuration JSON for legacy local Termux adapter mode.
func GenerateClientJSON(c *appconfig.Config) ([]byte, error) {
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
				"port":     10809,
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
				},
			},
		},
	}

	return json.MarshalIndent(clientConfig, "", "  ")
}
