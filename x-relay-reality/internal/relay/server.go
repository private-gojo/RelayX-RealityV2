package relay

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/config"
	"github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/relay/session"
	xrayprocess "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/xray/process"
)

func RunServer() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	maxSessions, err := session.MaxSessionsFromEnv()
	if err != nil {
		return err
	}

	configPath, cleanup, err := xrayprocess.WriteServerConfig(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := xrayprocess.TestConfig(cfg.XrayBin, configPath); err != nil {
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
		if err := xrayprocess.WaitForTCP(address, 10*time.Second); err != nil {
			_ = xrayCmd.Process.Kill()
			<-xrayDone
			return fmt.Errorf("Xray inbound %s did not become ready: %w", address, err)
		}
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           NewHTTPHandler(cfg, maxSessions),
		ReadHeaderTimeout: 10 * time.Second,
	}
	httpDone := make(chan error, 1)
	go func() {
		log.Printf("[SERVER] listening=:%s chain_path=%s legacy_path=%s profile_path=/profile.json max_sessions=%d", cfg.Port, cfg.ChainPath, cfg.WSPath, maxSessions)
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
		_ = httpServer.Close()
		return fmt.Errorf("Xray exited unexpectedly: %v", err)
	}
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
