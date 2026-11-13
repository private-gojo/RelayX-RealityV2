package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		UUID:        "47be59a9-103b-4065-9c3a-ebb9ecf8ffd3",
		PrivateKey:  FixedPrivateKey,
		ShortID:     "0123456789abcdef",
		ServerName:  FixedServerName,
		Target:      FixedTarget,
		Path:        "/reality",
		BridgeToken: strings.Repeat("a", 64),
		Port:        "8080",
		MaxSessions: 256,
	}
}

func decodeConfig(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	return decoded
}

func TestConfigValidate(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestConfigRejectsInvalidShortID(t *testing.T) {
	cfg := validConfig()
	cfg.ShortID = "abc"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected odd-length short ID to fail")
	}
}

func TestConfigRejectsInvalidPrivateKey(t *testing.T) {
	cfg := validConfig()
	cfg.PrivateKey = "not-a-key"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid private key to fail")
	}
}

func TestServerJSONDefinesChainAndREALITYInbounds(t *testing.T) {
	data, err := validConfig().ServerJSON()
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeConfig(t, data)
	inbounds := decoded["inbounds"].([]any)
	if len(inbounds) != 2 {
		t.Fatalf("expected two inbounds, got %d", len(inbounds))
	}

	chain := inbounds[0].(map[string]any)
	if chain["tag"] != "chain-ws" || chain["port"] != float64(ChainInboundPort) {
		t.Fatalf("unexpected chain inbound: %#v", chain)
	}
	chainStream := chain["streamSettings"].(map[string]any)
	if chainStream["method"] != "websocket" || chainStream["security"] != "none" {
		t.Fatalf("unexpected chain transport: %#v", chainStream)
	}
	chainWS := chainStream["wsSettings"].(map[string]any)
	if chainWS["path"] != ChainInboundPath {
		t.Fatalf("unexpected chain path: %#v", chainWS)
	}

	reality := inbounds[1].(map[string]any)
	if reality["tag"] != "reality-xhttp" || reality["port"] != float64(RealityInboundPort) {
		t.Fatalf("unexpected REALITY inbound: %#v", reality)
	}
	stream := reality["streamSettings"].(map[string]any)
	if stream["method"] != "xhttp" || stream["security"] != "reality" {
		t.Fatalf("unexpected REALITY transport settings: %#v", stream)
	}
	realitySettings := stream["realitySettings"].(map[string]any)
	if realitySettings["target"] != FixedTarget {
		t.Fatalf("unexpected REALITY target: %v", realitySettings["target"])
	}
	serverNames := realitySettings["serverNames"].([]any)
	if len(serverNames) != 1 || serverNames[0] != FixedServerName {
		t.Fatalf("unexpected REALITY serverNames: %#v", serverNames)
	}

	routing := decoded["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	rule := rules[0].(map[string]any)
	if rule["outboundTag"] != "chain-redirect" {
		t.Fatalf("unexpected chain routing rule: %#v", rule)
	}
	outbounds := decoded["outbounds"].([]any)
	redirect := outbounds[1].(map[string]any)
	settings := redirect["settings"].(map[string]any)
	if settings["redirect"] != "127.0.0.1:18443" {
		t.Fatalf("unexpected chain redirect: %#v", settings)
	}
}

func TestClientJSONPreservesLegacyAdapterProfile(t *testing.T) {
	data, err := validConfig().ClientJSON()
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeConfig(t, data)
	outbound := decoded["outbounds"].([]any)[0].(map[string]any)
	settings := outbound["settings"].(map[string]any)
	vnext := settings["vnext"].([]any)[0].(map[string]any)
	if vnext["address"] != "127.0.0.1" || vnext["port"] != float64(11000) {
		t.Fatalf("legacy client adapter target changed: %#v", vnext)
	}
	stream := outbound["streamSettings"].(map[string]any)
	reality := stream["realitySettings"].(map[string]any)
	if reality["password"] != FixedPublicKey || reality["serverName"] != FixedServerName {
		t.Fatalf("unexpected REALITY client settings: %#v", reality)
	}
	if _, exists := reality["publicKey"]; exists {
		t.Fatal("legacy publicKey field must not be emitted")
	}
}

func TestNetModJSONUsesDialerProxyChain(t *testing.T) {
	cfg := validConfig()
	data, err := cfg.NetModJSON("relay.example.run.app")
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeConfig(t, data)
	outbounds := decoded["outbounds"].([]any)
	if len(outbounds) != 2 {
		t.Fatalf("expected two client outbounds, got %d", len(outbounds))
	}

	realityOutbound := outbounds[0].(map[string]any)
	realityStream := realityOutbound["streamSettings"].(map[string]any)
	sockopt := realityStream["sockopt"].(map[string]any)
	if sockopt["dialerProxy"] != "cloudrun-chain" {
		t.Fatalf("REALITY outbound is not chained: %#v", sockopt)
	}
	if realityStream["method"] != "xhttp" || realityStream["security"] != "reality" {
		t.Fatalf("unexpected primary transport: %#v", realityStream)
	}

	chainOutbound := outbounds[1].(map[string]any)
	chainSettings := chainOutbound["settings"].(map[string]any)
	chainVnext := chainSettings["vnext"].([]any)[0].(map[string]any)
	if chainVnext["address"] != "relay.example.run.app" || chainVnext["port"] != float64(443) {
		t.Fatalf("unexpected Cloud Run endpoint: %#v", chainVnext)
	}
	chainStream := chainOutbound["streamSettings"].(map[string]any)
	if chainStream["method"] != "websocket" || chainStream["security"] != "tls" {
		t.Fatalf("unexpected outer transport: %#v", chainStream)
	}
	ws := chainStream["wsSettings"].(map[string]any)
	if ws["path"] != ChainPath || ws["host"] != "relay.example.run.app" {
		t.Fatalf("unexpected Cloud Run WebSocket settings: %#v", ws)
	}
	headers := ws["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer "+cfg.BridgeToken {
		t.Fatal("missing bridge authorization header")
	}
	tls := chainStream["tlsSettings"].(map[string]any)
	if tls["serverName"] != "relay.example.run.app" {
		t.Fatalf("unexpected Cloud Run TLS server name: %#v", tls)
	}
}

func TestNetModJSONRejectsInvalidCloudRunHost(t *testing.T) {
	for _, host := range []string{
		"",
		"https://relay.example.run.app",
		"relay.example.run.app/path",
		"bad host.example",
	} {
		if _, err := validConfig().NetModJSON(host); err == nil {
			t.Errorf("expected invalid host %q to be rejected", host)
		}
	}
}
