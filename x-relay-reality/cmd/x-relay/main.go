package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/bridge"
	"github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/config"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	if len(os.Args) < 2 {
		fatal("usage: x-relay server | client | client-config | validate-config")
	}

	var err error
	switch os.Args[1] {
	case "server":
		err = runServer()
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

func runServer() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	maxSessions, err := maxSessionsFromEnv()
	if err != nil {
		return err
	}

	configPath, cleanup, err := writeTempConfig(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := testXrayConfig(cfg.XrayBin, configPath); err != nil {
		return err
	}

	xrayCmd := exec.Command(cfg.XrayBin, "run", "-config", configPath)
	xrayCmd.Stdout = os.Stdout
	xrayCmd.Stderr = os.Stderr
	if err := xrayCmd.Start(); err != nil {
		return fmt.Errorf("start Xray: %w", err)
	}
	xrayDone := make(chan error, 1)
	go func() { xrayDone <- xrayCmd.Wait() }()

	for _, address := range []string{cfg.XrayWSAddr, cfg.XrayAddr} {
		if err := waitForTCP(address, 10*time.Second); err != nil {
			_ = xrayCmd.Process.Kill()
			<-xrayDone
			return fmt.Errorf("Xray inbound %s did not become ready: %w", address, err)
		}
	}

	bridgeServer := bridge.NewServer(cfg.XrayAddr, cfg.WSPath, cfg.RelayToken, maxSessions)
	mux := http.NewServeMux()
	mux.HandleFunc("/profile.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := cfg.WithCloudRunHost(r.Host).GenerateNetModProfileJSON()
		if err != nil {
			http.Error(w, "invalid service host", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
	mux.Handle("/", bridgeServer.Handler())

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	httpDone := make(chan error, 1)
	go func() {
		log.Printf("[SERVER] listening=:%s chain_path=%s legacy_path=%s profile_path=/profile.json max_sessions=%d",
			cfg.Port, cfg.ChainPath, cfg.WSPath, maxSessions)
		httpDone <- httpServer.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case sig := <-signals:
		log.Printf("[SERVER] received signal=%s", sig)
	case err := <-httpDone:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[SERVER] HTTP server stopped: %v", err)
		}
	case err := <-xrayDone:
		bridgeServer.CloseAll()
		_ = httpServer.Close()
		return fmt.Errorf("Xray exited unexpectedly: %v", err)
	}

	bridgeServer.CloseAll()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	_ = xrayCmd.Process.Signal(os.Interrupt)
	select {
	case <-xrayDone:
	case <-time.After(5 * time.Second):
		_ = xrayCmd.Process.Kill()
		<-xrayDone
	}
	return nil
}

func runClient() error {
	localAddr := valueOr(os.Getenv("LOCAL_ADDR"), "127.0.0.1:11000")
	remoteURL := os.Getenv("REMOTE_WS_URL")
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	maxSessions, err := maxSessionsFromEnv()
	if err != nil {
		return err
	}

	client, err := bridge.NewClient(localAddr, remoteURL, cfg.RelayToken, maxSessions)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return client.Run(ctx)
}

func writeClientConfig() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	data, err := cfg.GenerateClientJSON()
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
	serverConfig, err := cfg.GenerateServerJSON()
	if err != nil {
		return err
	}
	serverPath, cleanupServer, err := writeTempData(serverConfig)
	if err != nil {
		return err
	}
	defer cleanupServer()

	clientConfig, err := cfg.GenerateClientJSON()
	if err != nil {
		return err
	}
	clientPath, cleanupClient, err := writeTempData(clientConfig)
	if err != nil {
		return err
	}
	defer cleanupClient()

	netModConfig, err := cfg.WithCloudRunHost("smoke-test.run.app").GenerateNetModProfileJSON()
	if err != nil {
		return err
	}
	netModPath, cleanupNetMod, err := writeTempData(netModConfig)
	if err != nil {
		return err
	}
	defer cleanupNetMod()

	if err := testXrayConfig(cfg.XrayBin, serverPath); err != nil {
		return fmt.Errorf("server config validation failed: %w", err)
	}
	if err := testXrayConfig(cfg.XrayBin, clientPath); err != nil {
		return fmt.Errorf("legacy client config validation failed: %w", err)
	}
	if err := testXrayConfig(cfg.XrayBin, netModPath); err != nil {
		return fmt.Errorf("NetMod chained profile validation failed: %w", err)
	}
	log.Printf("[CHECK] server, legacy client, and zero-Termux NetMod configurations accepted")
	return nil
}

func writeTempConfig(cfg *config.Config) (string, func(), error) {
	data, err := cfg.GenerateServerJSON()
	if err != nil {
		return "", nil, err
	}
	return writeTempData(data)
}

func writeTempData(data []byte) (string, func(), error) {
	file, err := os.CreateTemp("", "x-relay-xray-*.json")
	if err != nil {
		return "", nil, err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, err
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, err
	}
	return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
}

func testXrayConfig(binary, path string) error {
	cmd := exec.Command(binary, "run", "-test", "-config", path)
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		log.Printf("[XRAY-TEST] %s", output)
	}
	if err != nil {
		return fmt.Errorf("Xray rejected generated configuration: %w", err)
	}
	return nil
}

func waitForTCP(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", address)
}

func maxSessionsFromEnv() (int, error) {
	value := valueOr(os.Getenv("MAX_SESSIONS"), "256")
	maxSessions, err := strconv.Atoi(value)
	if err != nil || maxSessions < 1 || maxSessions > 1000 {
		return 0, errors.New("MAX_SESSIONS must be an integer from 1 to 1000")
	}
	return maxSessions, nil
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func fatal(format string, args ...any) {
	log.Printf("[FATAL] "+format, args...)
	os.Exit(1)
}
