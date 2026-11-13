package server

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/bridge/connection"
	wsencap "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/encapsulation/websocket"
)

func NewServerHandler(targetAddr, expectedToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if expectedToken != "" {
			token := r.Header.Get("X-Relay-Auth")
			if token != expectedToken && r.Header.Get("Authorization") != "Bearer "+expectedToken {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		wsConn, err := wsencap.Upgrade(w, r)
		if err != nil {
			log.Printf("[server] websocket upgrade failed: %v", err)
			return
		}
		defer wsConn.Close()
		tcpConn, err := net.DialTimeout("tcp", targetAddr, wsencap.DialTimeout)
		if err != nil {
			log.Printf("[server] failed to dial local xray target %s: %v", targetAddr, err)
			return
		}
		defer tcpConn.Close()
		connection.BridgeConnections(wsConn, tcpConn)
	}
}

func NewWSProxyHandler(targetWSAddr, wsPath, expectedToken string) http.HandlerFunc {
	targetURL := &url.URL{Scheme:"http", Host:targetWSAddr}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.FlushInterval = -1
	return func(w http.ResponseWriter, r *http.Request) {
		if expectedToken != "" && r.Header.Get("X-Relay-Auth") != expectedToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if wsPath != "" { r.URL.Path = wsPath }
		proxy.ServeHTTP(w, r)
	}
}
