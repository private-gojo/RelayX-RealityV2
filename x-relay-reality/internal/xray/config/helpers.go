package xrayconfig

import (
	"fmt"
	"strings"
)

func primarySNI(serverNames []string) string {
	sni := "firebaseremoteconfigrealtime.googleapis.com"
	for _, name := range serverNames {
		if strings.Contains(name, "firebaseremoteconfigrealtime") {
			return name
		}
	}
	if len(serverNames) > 0 {
		return serverNames[0]
	}
	return sni
}

func splitHostPort(addr, defaultHost string, defaultPort int) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return defaultHost, defaultPort, nil
	}
	var port int
	_, err := fmt.Sscanf(parts[1], "%d", &port)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port in %q: %w", addr, err)
	}
	return parts[0], port, nil
}
