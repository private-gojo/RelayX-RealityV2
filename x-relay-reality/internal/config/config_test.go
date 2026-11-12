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

func TestServerJSONUsesFixedFirebaseREALITYDestination(t *testing.T) {
	data, err := validConfig().ServerJSON()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	inbound := decoded["inbounds"].([]any)[0].(map[string]any)
	stream := inbound["streamSettings"].(map[string]any)
	if stream["method"] != "xhttp" || stream["security"] != "reality" {
		t.Fatalf("unexpected transport settings: %#v", stream)
	}
	reality := stream["realitySettings"].(map[string]any)
	if reality["target"] != FixedTarget {
		t.Fatalf("unexpected REALITY target: %v", reality["target"])
	}
	serverNames := reality["serverNames"].([]any)
	if len(serverNames) != 1 || serverNames[0] != FixedServerName {
		t.Fatalf("unexpected REALITY serverNames: %#v", serverNames)
	}
	if _, exists := stream["network"]; exists {
		t.Fatal("legacy network field must not be emitted")
	}
}

func TestClientJSONUsesFixedFirebaseREALITYServerName(t *testing.T) {
	data, err := validConfig().ClientJSON()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	outbound := decoded["outbounds"].([]any)[0].(map[string]any)
	stream := outbound["streamSettings"].(map[string]any)
	reality := stream["realitySettings"].(map[string]any)
	if reality["password"] != FixedPublicKey {
		t.Fatalf("unexpected REALITY password/public key: %v", reality["password"])
	}
	if reality["serverName"] != FixedServerName {
		t.Fatalf("unexpected REALITY serverName: %v", reality["serverName"])
	}
	if _, exists := reality["publicKey"]; exists {
		t.Fatal("legacy publicKey field must not be emitted")
	}
}
