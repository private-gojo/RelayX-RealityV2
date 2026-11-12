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

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/reality"
	_, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err == nil {
		t.Fatal("unauthorized websocket unexpectedly connected")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected HTTP 401, got %#v", response)
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
