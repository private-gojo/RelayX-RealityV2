package websocket

import (
	"net/http"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
)

const (
	BufferSize     = 32 * 1024
	DialTimeout    = 10 * time.Second
	WriteWait      = 10 * time.Second
	PongWait       = 60 * time.Second
	PingPeriod     = (PongWait * 9) / 10
	MaxMessageSize = 64 * 1024
)

var upgrader = gorilla.Upgrader{
	ReadBufferSize:  BufferSize,
	WriteBufferSize: BufferSize,
	CheckOrigin: func(r *http.Request) bool { return true },
}

func Upgrade(w http.ResponseWriter, r *http.Request) (*gorilla.Conn, error) {
	return upgrader.Upgrade(w, r, nil)
}

func NewDialer() *gorilla.Dialer {
	return &gorilla.Dialer{
		HandshakeTimeout: DialTimeout,
		ReadBufferSize: BufferSize,
		WriteBufferSize: BufferSize,
	}
}

func ConfigureRead(conn *gorilla.Conn) {
	conn.SetReadLimit(MaxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(PongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(PongWait))
		return nil
	})
}

func WriteMessage(conn *gorilla.Conn, mu *sync.Mutex, messageType int, data []byte) error {
	mu.Lock()
	defer mu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(WriteWait))
	return conn.WriteMessage(messageType, data)
}
