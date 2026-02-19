package terminal

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/gorilla/websocket"
)

type WebSocketTerminal struct {
	conn   *websocket.Conn
	closed bool
	mu     sync.Mutex
}

type wsMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

func NewWebSocketTerminal(url, token string) (*WebSocketTerminal, error) {
	// Add token to headers
	headers := make(map[string][]string)
	headers["Authorization"] = []string{"Bearer " + token}

	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(url, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return &WebSocketTerminal{conn: conn}, nil
}

func (t *WebSocketTerminal) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	return t.conn.Close()
}

func (t *WebSocketTerminal) Resize(cols, rows int) error {
	msg := wsMessage{
		Type: "resize",
		Cols: cols,
		Rows: rows,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	return t.conn.WriteMessage(websocket.TextMessage, data)
}

func (t *WebSocketTerminal) Run(stdin io.Reader, stdout io.Writer) error {
	errChan := make(chan error, 2)

	// Read from WebSocket, write to stdout
	go func() {
		for {
			_, data, err := t.conn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}

			var msg wsMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				// Treat as raw data
				stdout.Write(data)
				continue
			}

			if msg.Type == "data" {
				stdout.Write([]byte(msg.Data))
			}
		}
	}()

	// Read from stdin, write to WebSocket
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdin.Read(buf)
			if err != nil {
				errChan <- err
				return
			}

			msg := wsMessage{
				Type: "data",
				Data: string(buf[:n]),
			}

			data, _ := json.Marshal(msg)

			t.mu.Lock()
			err = t.conn.WriteMessage(websocket.TextMessage, data)
			t.mu.Unlock()

			if err != nil {
				errChan <- err
				return
			}
		}
	}()

	return <-errChan
}
