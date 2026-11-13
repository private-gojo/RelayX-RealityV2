package bridge

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestUnauthorizedWebsocketRejected(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()

	server := NewServer(upstream.Addr().String(), "/reality", strings.Repeat("a", 64), 4)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	for _, path := range []string{"/reality", "/_ws", "/_chain"} {
		wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + path
		_, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if err == nil {
			t.Fatalf("unauthorized websocket unexpectedly connected on %s", path)
		}
		if response == nil || response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected HTTP 401 for %s, got %#v", path, response)
		}
	}
}

func TestWebsocketBridgePreservesByteStream(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()

	go func() {
		conn, acceptErr := upstream.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	token := strings.Repeat("b", 64)
	bridgeServer := NewServer(upstream.Addr().String(), "/reality", token, 4)
	httpServer := httptest.NewServer(bridgeServer.Handler())
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/reality"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	payload := []byte("split TCP stream across binary WebSocket frames")
	if err := ws.WriteMessage(websocket.BinaryMessage, payload[:9]); err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, payload[9:]); err != nil {
		t.Fatal(err)
	}

	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var received []byte
	for len(received) < len(payload) {
		messageType, message, err := ws.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if messageType != websocket.BinaryMessage {
			t.Fatalf("unexpected websocket message type: %d", messageType)
		}
		received = append(received, message...)
	}
	if string(received) != string(payload) {
		t.Fatalf("stream mismatch: got %q want %q", received, payload)
	}
}

func TestLegacyWebsocketAliasPreservesByteStream(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()

	go func() {
		conn, acceptErr := upstream.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	token := strings.Repeat("c", 64)
	server := NewServer(upstream.Addr().String(), "/reality", token, 4)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/_ws"
	header := http.Header{"Authorization": []string{"Bearer " + token}}
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	payload := []byte("legacy alias")
	if err := ws.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, received, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(received) != string(payload) {
		t.Fatalf("stream mismatch: got %q want %q", received, payload)
	}
}

func TestChainWebsocketProxiesFrames(t *testing.T) {
	token := strings.Repeat("d", 64)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	chainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chain" {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(messageType, payload); err != nil {
				return
			}
		}
	}))
	defer chainServer.Close()

	host := strings.TrimPrefix(chainServer.URL, "http://")
	server := NewServer("127.0.0.1:1", "/reality", token, 4)
	server.chainUpstream = host
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/_chain"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	payload := []byte("NetMod outer VLESS WebSocket frame")
	if err := ws.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	messageType, received, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.BinaryMessage || string(received) != string(payload) {
		t.Fatalf("unexpected proxied frame: type=%d payload=%q", messageType, received)
	}
}
