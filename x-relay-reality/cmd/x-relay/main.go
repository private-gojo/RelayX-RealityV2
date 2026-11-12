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
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	configPath, cleanup, err := writeTempConfig(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	xrayBin := valueOr(os.Getenv("XRAY_BIN"), "/usr/local/bin/xray")
	if err := testXrayConfig(xrayBin, configPath); err != nil {
		return err
	}

	xrayCmd := exec.Command(xrayBin, "run", "-config", configPath)
	xrayCmd.Stdout = os.Stdout
	xrayCmd.Stderr = os.Stderr
	if err := xrayCmd.Start(); err != nil {
		return fmt.Errorf("start Xray: %w", err)
	}
	xrayDone := make(chan error, 1)
	go func() { xrayDone <- xrayCmd.Wait() }()

	if err := waitForTCP("127.0.0.1:10000", 10*time.Second); err != nil {
		_ = xrayCmd.Process.Kill()
		<-xrayDone
		return fmt.Errorf("Xray inbound did not become ready: %w", err)
	}

	bridgeServer := bridge.NewServer("127.0.0.1:10000", cfg.Path, cfg.BridgeToken, cfg.MaxSessions)
	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           bridgeServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	httpDone := make(chan error, 1)
	go func() {
		log.Printf("[SERVER] listening=:%s path=%s max_sessions=%d", cfg.Port, cfg.Path, cfg.MaxSessions)
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
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	token := cfg.BridgeToken
	maxSessions := 256
	if value := os.Getenv("MAX_SESSIONS"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid MAX_SESSIONS: %w", err)
		}
		maxSessions = n
	}
	client, err := bridge.NewClient(localAddr, remoteURL, token, maxSessions)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return client.Run(ctx)
}

func writeClientConfig() error {
	cfg, err := config.LoadClient()
	if err != nil {
		return err
	}
	data, err := cfg.ClientJSON()
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
	log.Printf("[CLIENT] Xray config written to %s", output)
	return nil
}

func validateXrayConfig() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	serverConfig, err := cfg.ServerJSON()
	if err != nil {
		return err
	}
	serverPath, cleanupServer, err := writeTempData(serverConfig)
	if err != nil {
		return err
	}
	defer cleanupServer()

	clientConfig, err := cfg.ClientJSON()
	if err != nil {
		return err
	}
	clientPath, cleanupClient, err := writeTempData(clientConfig)
	if err != nil {
		return err
	}
	defer cleanupClient()

	xrayBin := valueOr(os.Getenv("XRAY_BIN"), "/usr/local/bin/xray")
	if err := testXrayConfig(xrayBin, serverPath); err != nil {
		return fmt.Errorf("server config validation failed: %w", err)
	}
	if err := testXrayConfig(xrayBin, clientPath); err != nil {
		return fmt.Errorf("client config validation failed: %w", err)
	}
	log.Printf("[CHECK] server and client Xray configurations accepted")
	return nil
}

func writeTempConfig(cfg config.Config) (string, func(), error) {
	data, err := cfg.ServerJSON()
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
