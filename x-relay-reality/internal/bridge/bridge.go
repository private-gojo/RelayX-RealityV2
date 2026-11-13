package bridge

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 90 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 4 << 20
	chainUpstream  = "127.0.0.1:18080"
	chainPath      = "/_chain"
)

type Server struct {
	upstream      string
	chainUpstream string
	path          string
	token         string
	limit         chan struct{}
	upgrader      websocket.Upgrader
	chainDialer   websocket.Dialer
	mu            sync.Mutex
	active        map[*websocket.Conn]struct{}
}

func NewServer(upstream, path, token string, maxSessions int) *Server {
	return &Server{
		upstream:      upstream,
		chainUpstream: chainUpstream,
		path:          path,
		token:         token,
		limit:         make(chan struct{}, maxSessions),
		active:        make(map[*websocket.Conn]struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:    4096,
			WriteBufferSize:   4096,
			EnableCompression: false,
			CheckOrigin: func(r *http.Request) bool {
				return r.Header.Get("Origin") == ""
			},
		},
		chainDialer: websocket.Dialer{
			HandshakeTimeout:  5 * time.Second,
			EnableCompression: false,
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc(chainPath, s.handleChain)
	mux.HandleFunc("/_ws", s.handleTunnel)
	mux.HandleFunc(s.path, s.handleTunnel)
	return mux
}

func (s *Server) handleTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !authorized(r.Header.Get("Authorization"), s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !s.acquire(w) {
		return
	}
	defer func() { <-s.limit }()

	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.track(ws, true)
	defer s.track(ws, false)
	defer ws.Close()

	dialer := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	tcp, err := dialer.DialContext(r.Context(), "tcp", s.upstream)
	if err != nil {
		_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "upstream unavailable"), time.Now().Add(writeWait))
		return
	}
	defer tcp.Close()
	relay(tcp, ws)
}

func (s *Server) handleChain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !authorized(r.Header.Get("Authorization"), s.token) &&
		!authorized("Bearer "+r.Header.Get("X-Relay-Auth"), s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !s.acquire(w) {
		return
	}
	defer func() { <-s.limit }()

	client, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.track(client, true)
	defer s.track(client, false)
	defer client.Close()

	headers := http.Header{}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		headers.Set("X-Forwarded-For", forwarded)
	}
	upstreamURL := "ws://" + s.chainUpstream + chainPathInternal
	upstream, response, err := s.chainDialer.Dial(upstreamURL, headers)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		_ = client.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "chain upstream unavailable"),
			time.Now().Add(writeWait),
		)
		return
	}
	defer upstream.Close()
	relayWebSockets(client, upstream)
}

const chainPathInternal = "/chain"

func (s *Server) acquire(w http.ResponseWriter) bool {
	select {
	case s.limit <- struct{}{}:
		return true
	default:
		http.Error(w, "session capacity reached", http.StatusServiceUnavailable)
		return false
	}
}

func (s *Server) track(conn *websocket.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if add {
		s.active[conn] = struct{}{}
		return
	}
	delete(s.active, conn)
}

func (s *Server) CloseAll() {
	s.mu.Lock()
	connections := make([]*websocket.Conn, 0, len(s.active))
	for conn := range s.active {
		connections = append(connections, conn)
	}
	s.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}

type Client struct {
	localAddr string
	remoteURL string
	token     string
	limit     chan struct{}
	dialer    websocket.Dialer
}

func NewClient(localAddr, remoteURL, token string, maxSessions int) (*Client, error) {
	u, err := url.Parse(remoteURL)
	if err != nil || u.Scheme != "wss" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("REMOTE_WS_URL must be a wss:// URL without userinfo or fragment")
	}
	host, _, err := net.SplitHostPort(localAddr)
	if err != nil || host == "" {
		return nil, errors.New("LOCAL_ADDR must include a host and port")
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("LOCAL_ADDR must bind to loopback only")
	}
	if len(token) < 32 {
		return nil, errors.New("BRIDGE_TOKEN must be at least 32 characters")
	}
	if maxSessions < 1 || maxSessions > 1000 {
		return nil, errors.New("MAX_SESSIONS must be from 1 to 1000")
	}
	return &Client{
		localAddr: localAddr,
		remoteURL: remoteURL,
		token:     token,
		limit:     make(chan struct{}, maxSessions),
		dialer: websocket.Dialer{
			Proxy:             http.ProxyFromEnvironment,
			HandshakeTimeout:  15 * time.Second,
			EnableCompression: false,
		},
	}, nil
}

func (c *Client) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", c.localAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", c.localAddr, err)
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	log.Printf("[CLIENT] listening=%s remote=%s", c.localAddr, c.remoteURL)
	for {
		localConn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept local connection: %w", err)
		}
		select {
		case c.limit <- struct{}{}:
			go func() {
				defer func() { <-c.limit }()
				c.handleLocal(localConn)
			}()
		default:
			_ = localConn.Close()
		}
	}
}

func (c *Client) handleLocal(localConn net.Conn) {
	defer localConn.Close()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.token)
	ws, response, err := c.dialer.Dial(c.remoteURL, header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		log.Printf("[CLIENT] websocket connect failed: %v", err)
		return
	}
	defer ws.Close()
	relay(localConn, ws)
}

func authorized(header, token string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	presented := strings.TrimPrefix(header, prefix)
	return len(presented) == len(token) &&
		subtle.ConstantTimeCompare([]byte(presented), []byte(token)) == 1
}

func relay(tcp net.Conn, ws *websocket.Conn) {
	ws.SetReadLimit(maxMessageSize)
	_ = ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	stopPing := make(chan struct{})
	pingDone := make(chan struct{})
	go func() {
		defer close(pingDone)
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
					return
				}
			}
		}
	}()

	done := make(chan struct{}, 2)
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			n, err := tcp.Read(buffer)
			if n > 0 {
				_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
				if writeErr := ws.WriteMessage(websocket.BinaryMessage, buffer[:n]); writeErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		done <- struct{}{}
	}()

	go func() {
		for {
			messageType, payload, err := ws.ReadMessage()
			if err != nil || messageType != websocket.BinaryMessage {
				break
			}
			if err := writeAll(tcp, payload); err != nil {
				break
			}
		}
		done <- struct{}{}
	}()

	<-done
	_ = tcp.Close()
	_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(writeWait))
	_ = ws.Close()
	<-done
	close(stopPing)
	<-pingDone
}

func relayWebSockets(client, upstream *websocket.Conn) {
	done := make(chan struct{}, 2)
	pump := func(dst, src *websocket.Conn) {
		defer func() { done <- struct{}{} }()
		src.SetReadLimit(maxMessageSize)
		for {
			messageType, payload, err := src.ReadMessage()
			if err != nil {
				return
			}
			_ = dst.SetWriteDeadline(time.Now().Add(writeWait))
			if err := dst.WriteMessage(messageType, payload); err != nil {
				return
			}
		}
	}
	go pump(upstream, client)
	go pump(client, upstream)
	<-done
	_ = client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(writeWait))
	_ = upstream.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(writeWait))
	_ = client.Close()
	_ = upstream.Close()
	<-done
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
