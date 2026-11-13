package connection

import (
	"io"
	"log"
	"net"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
	wsencap "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/encapsulation/websocket"
)

func BridgeConnections(wsConn *gorilla.Conn, tcpConn net.Conn) {
	wsencap.ConfigureRead(wsConn)
	var writeMu sync.Mutex
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
	go func() {
		defer wg.Done()
		defer closeBoth()
		for {
			messageType, data, err := wsConn.ReadMessage()
			if err != nil {
				return
			}
			if messageType != gorilla.BinaryMessage && messageType != gorilla.TextMessage {
				continue
			}
			if _, err := tcpConn.Write(data); err != nil {
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		defer closeBoth()
		buf := make([]byte, wsencap.BufferSize)
		for {
			n, err := tcpConn.Read(buf)
			if n > 0 {
				if writeErr := wsencap.WriteMessage(wsConn, &writeMu, gorilla.BinaryMessage, buf[:n]); writeErr != nil {
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
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(wsencap.PingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := wsencap.WriteMessage(wsConn, &writeMu, gorilla.PingMessage, nil); err != nil {
					closeBoth()
					return
				}
			}
		}
	}()
	wg.Wait()
}
