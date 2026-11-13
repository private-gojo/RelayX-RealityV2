package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	bridgeclient "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/bridge/client"
	"github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/config"
	"github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/relay"
	"github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/relay/session"
	xrayconfig "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/xray/config"
	xrayprocess "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/xray/process"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	if len(os.Args) < 2 {
		fatal("usage: x-relay server | client | client-config | validate-config")
	}
	var err error
	switch os.Args[1] {
	case "server":
		err = relay.RunServer()
	case "client":
		err = runClient()
	case "client-config":
		err = writeClientConfig()
	case "validate-config":
		err = validateXrayConfig()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatal("%v", err)
	}
}

func runClient() error {
	localAddr := valueOr(os.Getenv("LOCAL_ADDR"), "127.0.0.1:11000")
	remoteURL := os.Getenv("REMOTE_WS_URL")
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	maxSessions, err := session.MaxSessionsFromEnv()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return bridgeclient.RunClient(ctx, localAddr, remoteURL, "", cfg.RelayToken, maxSessions)
}

func writeClientConfig() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	data, err := xrayconfig.GenerateClientJSON(cfg)
	if err != nil {
		return err
	}
	output := valueOr(os.Getenv("CLIENT_CONFIG_OUT"), "client/xray-client.json")
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(output, data, 0600); err != nil {
		return err
	}
	log.Printf("[CLIENT] legacy adapter Xray config written to %s", output)
	return nil
}

func validateXrayConfig() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	serverConfig, err := xrayconfig.GenerateServerJSON(cfg)
	if err != nil {
		return err
	}
	serverPath, cleanupServer, err := xrayprocess.WriteTempData(serverConfig)
	if err != nil {
		return err
	}
	defer cleanupServer()
	clientConfig, err := xrayconfig.GenerateClientJSON(cfg)
	if err != nil {
		return err
	}
	clientPath, cleanupClient, err := xrayprocess.WriteTempData(clientConfig)
	if err != nil {
		return err
	}
	defer cleanupClient()
	netModConfig, err := xrayconfig.GenerateNetModProfileJSON(cfg.WithCloudRunHost("smoke-test.run.app"))
	if err != nil {
		return err
	}
	netModPath, cleanupNetMod, err := xrayprocess.WriteTempData(netModConfig)
	if err != nil {
		return err
	}
	defer cleanupNetMod()
	if err := xrayprocess.TestConfig(cfg.XrayBin, serverPath); err != nil {
		return fmt.Errorf("server config validation failed: %w", err)
	}
	if err := xrayprocess.TestConfig(cfg.XrayBin, clientPath); err != nil {
		return fmt.Errorf("legacy client config validation failed: %w", err)
	}
	if err := xrayprocess.TestConfig(cfg.XrayBin, netModPath); err != nil {
		return fmt.Errorf("NetMod chained profile validation failed: %w", err)
	}
	log.Printf("[CHECK] server, legacy client, and zero-Termux NetMod configurations accepted")
	return nil
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func fatal(format string, args ...any) { log.Printf("[FATAL] "+format, args...); os.Exit(1) }
