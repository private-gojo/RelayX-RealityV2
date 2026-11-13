package config

import (
	"os"
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
