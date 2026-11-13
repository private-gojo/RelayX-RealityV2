package xrayconfig

func serverRouting() map[string]any {
	return map[string]any{
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
	}
}
