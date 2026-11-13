package xrayconfig

import (
	"encoding/json"
	"errors"
	"fmt"

	appconfig "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/config"
)

// GenerateServerJSON returns the Xray server configuration JSON bytes.
func GenerateServerJSON(c *appconfig.Config) ([]byte, error) {
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
		"inbounds": serverInbounds(c, host, portStr, wsHost, wsPort, chainPath),
		"outbounds": serverOutbounds(host, portStr),
		"routing": serverRouting(),
	}

	return json.MarshalIndent(serverConfig, "", "  ")
}

