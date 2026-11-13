package client

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/bridge/connection"
	wsencap "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/encapsulation/websocket"
)

func RunClient(ctx context.Context, listenAddr, wsURL, hostHeader, relayToken string, maxSessions int) error {
	if maxSessions < 1 || maxSessions > 1000 { return fmt.Errorf("maxSessions must be from 1 to 1000") }
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", listenAddr)
	if err != nil { return err }
	defer listener.Close()
	log.Printf("[client] listening on %s -> forwarding to %s", listenAddr, wsURL)
	go func() { <-ctx.Done(); _ = listener.Close() }()

	limit := make(chan struct{}, maxSessions)
	for {
		tcpConn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done(): return nil
			default: log.Printf("[client] accept error: %v", err); continue
			}
		}
		select {
		case limit <- struct{}{}:
			go func(conn net.Conn) {
				defer func(){ <-limit }()
				handleClientConn(ctx, conn, wsURL, hostHeader, relayToken)
			}(tcpConn)
		default: _ = tcpConn.Close()
		}
	}
}

func handleClientConn(ctx context.Context, tcpConn net.Conn, wsURL, hostHeader, relayToken string) {
	defer tcpConn.Close()
	headers := http.Header{}
	if hostHeader != "" { headers.Set("Host", hostHeader) }
	if relayToken != "" { headers.Set("X-Relay-Auth", relayToken) }
	wsConn, resp, err := wsencap.NewDialer().DialContext(ctx, wsURL, headers)
	if err != nil {
		if resp != nil { log.Printf("[client] ws dial failed (HTTP %d): %v", resp.StatusCode, err) } else { log.Printf("[client] ws dial failed: %v", err) }
		return
	}
	defer wsConn.Close()
	connection.BridgeConnections(wsConn, tcpConn)
}
