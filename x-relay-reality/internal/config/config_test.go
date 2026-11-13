package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestLoadFromEnvServer(t *testing.T) {
	os.Setenv("MODE", "server")
	os.Setenv("PORT", "9090")
	os.Setenv("VLESS_UUID", "11111111-2222-3333-4444-555555555555")
	defer func() {
		os.Unsetenv("MODE")
		os.Unsetenv("PORT")
		os.Unsetenv("VLESS_UUID")
	}()

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mode != "server" {
		t.Errorf("expected mode 'server', got %q", cfg.Mode)
	}
	if cfg.Port != "9090" {
		t.Errorf("expected port '9090', got %q", cfg.Port)
	}
	if cfg.UUID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("unexpected UUID: %q", cfg.UUID)
	}
	if cfg.RealityDest != "firebaseremoteconfigrealtime.googleapis.com:443" {
		t.Errorf("unexpected default RealityDest: %q", cfg.RealityDest)
	}
}

func TestLoadFromEnvClientRequiresHost(t *testing.T) {
	os.Setenv("MODE", "client")
	os.Unsetenv("CLOUD_RUN_HOST")
	defer os.Unsetenv("MODE")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when CLOUD_RUN_HOST is missing in client mode")
	}
}

func TestGenerateServerJSON(t *testing.T) {
	cfg := &Config{
		Mode:        "server",
		XrayAddr:    "127.0.0.1:18443",
		XrayWSAddr:  "127.0.0.1:18080",
		ChainPath:   "/_chain",
		XHTTPPath:   "/xh",
		UUID:        "1ef40695-9d5b-498e-aeaa-c44ab281ca61",
		PrivateKey:  "eKS-gh5AXjEdJ3sGF7hDpUhFlKGJ615192I4no0yFGo",
		ShortID:     "6ca30cd71da67803",
		RealityDest: "firebaseremoteconfigrealtime.googleapis.com:443",
		ServerNames: []string{"googleapis.com", "firebaseremoteconfigrealtime.googleapis.com"},
	}

	data, err := cfg.GenerateServerJSON()
	if err != nil {
		t.Fatalf("GenerateServerJSON failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated JSON is invalid: %v", err)
	}

	inbounds, ok := parsed["inbounds"].([]any)
	if !ok || len(inbounds) != 2 {
		t.Fatalf("expected 2 inbounds (outer-ws-in + reality-xhttp-in), got %v", parsed["inbounds"])
	}

	s := string(data)
	for _, want := range []string{
		`"outer-ws-in"`,
		`"reality-xhttp-in"`,
		`"loopback-to-reality"`,
		`"redirect": "127.0.0.1:18443"`,
		`"/_chain"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("expected server JSON to contain %s", want)
		}
	}
}

func TestGenerateNetModProfileJSON(t *testing.T) {
	cfg := &Config{
		XrayAddr:     "127.0.0.1:18443",
		ChainPath:    "/_chain",
		XHTTPPath:    "/xh",
		CloudRunHost: "relayx-reality-v2-667648148327.asia-east1.run.app",
		RelayToken:   "relayx-secret-2026",
		UUID:         DefaultBakedUUID,
		PublicKey:    DefaultBakedPublicKey,
		ShortID:      DefaultBakedShortID,
		ServerNames:  []string{"googleapis.com", "firebaseremoteconfigrealtime.googleapis.com"},
	}

	data, err := cfg.GenerateNetModProfileJSON()
	if err != nil {
		t.Fatalf("GenerateNetModProfileJSON failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated NetMod profile JSON is invalid: %v", err)
	}

	s := string(data)
	for _, want := range []string{
		`"dialerProxy": "outer-ws"`,
		`"tag": "outer-ws"`,
		`"address": "firebaseremoteconfigrealtime.googleapis.com"`,
		`"Host": "relayx-reality-v2-667648148327.asia-east1.run.app"`,
		`"path": "/_chain"`,
		`"X-Relay-Auth": "relayx-secret-2026"`,
		`"serverName": "firebaseremoteconfigrealtime.googleapis.com"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("expected NetMod profile JSON to contain %s", want)
		}
	}
}
