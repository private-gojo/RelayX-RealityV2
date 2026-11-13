package process

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	appconfig "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/config"
	xrayconfig "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/xray/config"
)

func WriteServerConfig(cfg *appconfig.Config) (string, func(), error) {
	data, err := xrayconfig.GenerateServerJSON(cfg)
	if err != nil { return "", nil, err }
	return WriteTempData(data)
}

func WriteTempData(data []byte) (string, func(), error) {
	file, err := os.CreateTemp("", "x-relay-xray-*.json")
	if err != nil { return "", nil, err }
	if _, err := file.Write(data); err != nil {
		_ = file.Close(); _ = os.Remove(file.Name()); return "", nil, err
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close(); _ = os.Remove(file.Name()); return "", nil, err
	}
	if err := file.Close(); err != nil { _ = os.Remove(file.Name()); return "", nil, err }
	return file.Name(), func(){ _ = os.Remove(file.Name()) }, nil
}

func TestConfig(binary, path string) error {
	cmd := exec.Command(binary, "run", "-test", "-config", path)
	output, err := cmd.CombinedOutput()
	if len(output)>0 { log.Printf("[XRAY-TEST] %s", output) }
	if err != nil { return fmt.Errorf("Xray rejected generated configuration: %w", err) }
	return nil
}

func WaitForTCP(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
		if err == nil { _ = conn.Close(); return nil }
		time.Sleep(100*time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", address)
}
