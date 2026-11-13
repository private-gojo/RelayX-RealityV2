package xrayconfig

func serverOutbounds(host string, portStr int) []any {
	return []any{
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
		}
}
