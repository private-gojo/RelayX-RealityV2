package bridge

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	bufferSize     = 32 * 1024
	dialTimeout    = 10 * time.Second
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 64 * 1024
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  bufferSize,
	WriteBufferSize: bufferSize,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// NewServerHandler returns an http.HandlerFunc that upgrades incoming HTTP requests
// to WebSocket and bridges them to the local TCP targetAddr (Xray server).
func NewServerHandler(targetAddr, expectedToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if expectedToken != "" {
			token := r.Header.Get("X-Relay-Auth")
			if token != expectedToken && r.Header.Get("Authorization") != "Bearer "+expectedToken {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[server] websocket upgrade failed: %v", err)
			return
		}
		defer wsConn.Close()

		tcpConn, err := net.DialTimeout("tcp", targetAddr, dialTimeout)
		if err != nil {
			log.Printf("[server] failed to dial local xray target %s: %v", targetAddr, err)
			return
		}
		defer tcpConn.Close()

		bridgeConnections(wsConn, tcpConn)
	}
}

// NewWSProxyHandler forwards incoming HTTP/WebSocket upgrade requests directly to a local
// WebSocket backend (such as Xray's VLESS+WS inbound) using Go's native ReverseProxy
// with immediate streaming flush, preserving all headers, subprotocols, and framing.
func NewWSProxyHandler(targetWSAddr, wsPath, expectedToken string) http.HandlerFunc {
	targetURL := &url.URL{
		Scheme: "http",
		Host:   targetWSAddr,
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.FlushInterval = -1

	return func(w http.ResponseWriter, r *http.Request) {
		if expectedToken != "" {
			if r.Header.Get("X-Relay-Auth") != expectedToken {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		if wsPath != "" {
			r.URL.Path = wsPath
		}
		proxy.ServeHTTP(w, r)
	}
}

// RunClient starts a local TCP listener on listenAddr and forwards each connection
// over WebSocket (wsURL) to the Cloud Run server.
func RunClient(ctx context.Context, listenAddr, wsURL, hostHeader, relayToken string, maxSessions int) error {
	if maxSessions < 1 || maxSessions > 1000 {
		return fmt.Errorf("maxSessions must be from 1 to 1000")
	}

	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", listenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("[client] listening on %s -> forwarding to %s", listenAddr, wsURL)

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	limit := make(chan struct{}, maxSessions)
	for {
		tcpConn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				log.Printf("[client] accept error: %v", err)
				continue
			}
		}

		select {
		case limit <- struct{}{}:
			go func(conn net.Conn) {
				defer func() { <-limit }()
				handleClientConn(ctx, conn, wsURL, hostHeader, relayToken)
			}(tcpConn)
		default:
			_ = tcpConn.Close()
		}
	}
}

func handleClientConn(ctx context.Context, tcpConn net.Conn, wsURL, hostHeader, relayToken string) {
	defer tcpConn.Close()

	headers := http.Header{}
	if hostHeader != "" {
		headers.Set("Host", hostHeader)
	}
	if relayToken != "" {
		headers.Set("X-Relay-Auth", relayToken)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: dialTimeout,
		ReadBufferSize:   bufferSize,
		WriteBufferSize:  bufferSize,
	}

	wsConn, resp, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		if resp != nil {
			log.Printf("[client] ws dial failed (HTTP %d): %v", resp.StatusCode, err)
		} else {
			log.Printf("[client] ws dial failed: %v", err)
		}
		return
	}
	defer wsConn.Close()

	bridgeConnections(wsConn, tcpConn)
}

// bridgeConnections copies data bidirectionally between a WebSocket connection and a TCP connection.
func bridgeConnections(wsConn *websocket.Conn, tcpConn net.Conn) {
	wsConn.SetReadLimit(maxMessageSize)
	_ = wsConn.SetReadDeadline(time.Now().Add(pongWait))
	wsConn.SetPongHandler(func(string) error {
		_ = wsConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	var writeMu sync.Mutex
	writeWS := func(messageType int, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = wsConn.SetWriteDeadline(time.Now().Add(writeWait))
		return wsConn.WriteMessage(messageType, data)
	}

	done := make(chan struct{})
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			close(done)
			_ = wsConn.Close()
			_ = tcpConn.Close()
		})
	}

	var wg sync.WaitGroup
	wg.Add(3)

	// 1. WebSocket -> TCP
	go func() {
		defer wg.Done()
		defer closeBoth()
		for {
			msgType, data, err := wsConn.ReadMessage()
			if err != nil {
				return
			}
			if msgType != websocket.BinaryMessage && msgType != websocket.TextMessage {
				continue
			}
			if _, err := tcpConn.Write(data); err != nil {
				return
			}
		}
	}()

	// 2. TCP -> WebSocket
	go func() {
		defer wg.Done()
		defer closeBoth()
		buf := make([]byte, bufferSize)
		for {
			n, err := tcpConn.Read(buf)
			if n > 0 {
				if wErr := writeWS(websocket.BinaryMessage, buf[:n]); wErr != nil {
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("[bridge] tcp read error: %v", err)
				}
				return
			}
		}
	}()

	// 3. Ping ticker to keep Cloud Run connection alive
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := writeWS(websocket.PingMessage, nil); err != nil {
					closeBoth()
					return
				}
			}
		}
	}()

	wg.Wait()
}
