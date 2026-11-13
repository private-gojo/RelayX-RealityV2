package xrayconfig

import appconfig "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/config"

func serverInbounds(c *appconfig.Config, host string, portStr int, wsHost string, wsPort int, chainPath string) []any {
	return []any{
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
					"mode": "stream-one",
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
	}
}
