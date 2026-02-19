package terminal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type SocketIOTerminal struct {
	conn      *websocket.Conn
	namespace string
	sandboxID string

	mu       sync.Mutex
	closed   bool
	sessionM sync.RWMutex
	session  string
}

type terminalStartedPayload struct {
	SessionID string `json:"sessionId"`
}

type terminalOutputPayload struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
}

type terminalEndedPayload struct {
	SessionID string `json:"sessionId"`
}

type terminalErrorPayload struct {
	Message string `json:"message"`
}

type terminalInputPayload struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
}

type terminalResizePayload struct {
	SessionID string `json:"sessionId"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

func NewSocketIOTerminal(rawURL, token, sandboxID string) (*SocketIOTerminal, error) {
	engineURL, namespace, err := buildSocketIOURL(rawURL, token)
	if err != nil {
		return nil, err
	}

	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(engineURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	term := &SocketIOTerminal{
		conn:      conn,
		namespace: namespace,
		sandboxID: sandboxID,
	}

	if err := term.handshake(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return term, nil
}

func buildSocketIOURL(rawURL, token string) (string, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid websocket url: %w", err)
	}

	namespace := parsed.Path
	if namespace == "" {
		namespace = "/terminal"
	}

	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	}

	query := parsed.Query()
	query.Set("EIO", "4")
	query.Set("transport", "websocket")
	query.Set("token", token)

	parsed.Path = "/socket.io/"
	parsed.RawQuery = query.Encode()

	return parsed.String(), namespace, nil
}

func (t *SocketIOTerminal) namespacePrefix() string {
	if t.namespace == "" || t.namespace == "/" {
		return ""
	}
	return t.namespace + ","
}

func (t *SocketIOTerminal) handshake() error {
	// Engine.IO open packet
	if _, msg, err := t.conn.ReadMessage(); err != nil {
		return fmt.Errorf("socket.io handshake failed: %w", err)
	} else if len(msg) == 0 || msg[0] != '0' {
		return fmt.Errorf("socket.io handshake failed: unexpected open packet")
	}

	// Socket.IO namespace connect
	connectPacket := "40" + t.namespacePrefix()
	if err := t.writePacket(connectPacket); err != nil {
		return fmt.Errorf("socket.io namespace connect failed: %w", err)
	}

	return nil
}

func (t *SocketIOTerminal) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	return t.conn.Close()
}

func (t *SocketIOTerminal) setSessionID(sessionID string) {
	t.sessionM.Lock()
	defer t.sessionM.Unlock()
	t.session = sessionID
}

func (t *SocketIOTerminal) getSessionID() string {
	t.sessionM.RLock()
	defer t.sessionM.RUnlock()
	return t.session
}

func (t *SocketIOTerminal) Resize(cols, rows int) error {
	sessionID := t.getSessionID()
	if sessionID == "" {
		// Ignore early resize before terminal session starts.
		return nil
	}

	return t.emit("terminal:resize", terminalResizePayload{
		SessionID: sessionID,
		Cols:      cols,
		Rows:      rows,
	})
}

func (t *SocketIOTerminal) emit(event string, payload any) error {
	frameData, err := json.Marshal([]any{event, payload})
	if err != nil {
		return err
	}

	packet := "42" + t.namespacePrefix() + string(frameData)
	return t.writePacket(packet)
}

func (t *SocketIOTerminal) writePacket(packet string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return io.EOF
	}

	return t.conn.WriteMessage(websocket.TextMessage, []byte(packet))
}

func (t *SocketIOTerminal) Run(stdin io.Reader, stdout io.Writer) error {
	errChan := make(chan error, 2)
	started := make(chan struct{})
	var startOnce sync.Once

	if err := t.emit("terminal:start", map[string]string{
		"sandboxId": t.sandboxID,
	}); err != nil {
		return fmt.Errorf("failed to start terminal: %w", err)
	}

	go func() {
		for {
			_, data, err := t.conn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}

			packet := string(data)
			if len(packet) == 0 {
				continue
			}

			switch packet[0] {
			case '2':
				if err := t.writePacket("3"); err != nil {
					errChan <- err
					return
				}
				continue
			case '4':
				event, payload, ok := parseSocketIOEvent(packet[1:])
				if !ok {
					continue
				}

				switch event {
				case "terminal:started":
					var p terminalStartedPayload
					if err := json.Unmarshal(payload, &p); err != nil || p.SessionID == "" {
						errChan <- fmt.Errorf("failed to decode terminal:started payload")
						return
					}
					t.setSessionID(p.SessionID)
					startOnce.Do(func() { close(started) })
				case "terminal:output":
					var p terminalOutputPayload
					if err := json.Unmarshal(payload, &p); err != nil {
						continue
					}
					decoded, err := base64.StdEncoding.DecodeString(p.Data)
					if err != nil {
						_, _ = stdout.Write([]byte(p.Data))
						continue
					}
					_, _ = stdout.Write(decoded)
				case "terminal:error":
					var p terminalErrorPayload
					if err := json.Unmarshal(payload, &p); err != nil || strings.TrimSpace(p.Message) == "" {
						errChan <- fmt.Errorf("terminal error")
						return
					}
					errChan <- fmt.Errorf("terminal error: %s", p.Message)
					return
				case "terminal:ended":
					var p terminalEndedPayload
					_ = json.Unmarshal(payload, &p)
					errChan <- io.EOF
					return
				}
			}
		}
	}()

	select {
	case <-started:
	case err := <-errChan:
		if err == io.EOF {
			return nil
		}
		return err
	}

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdin.Read(buf)
			if err != nil {
				errChan <- err
				return
			}

			if err := t.emit("terminal:input", terminalInputPayload{
				SessionID: t.getSessionID(),
				Data:      base64.StdEncoding.EncodeToString(buf[:n]),
			}); err != nil {
				errChan <- err
				return
			}
		}
	}()

	err := <-errChan
	if err == io.EOF {
		return nil
	}
	return err
}

func parseSocketIOEvent(packet string) (string, json.RawMessage, bool) {
	// Socket.IO event packets are type "2", optionally followed by namespace and comma.
	if packet == "" || packet[0] != '2' {
		return "", nil, false
	}

	body := packet[1:]
	if strings.HasPrefix(body, "/") {
		idx := strings.Index(body, ",")
		if idx < 0 {
			return "", nil, false
		}
		body = body[idx+1:]
	}

	if body == "" || body[0] != '[' {
		return "", nil, false
	}

	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(body), &arr); err != nil || len(arr) == 0 {
		return "", nil, false
	}

	var event string
	if err := json.Unmarshal(arr[0], &event); err != nil || event == "" {
		return "", nil, false
	}

	if len(arr) > 1 {
		return event, arr[1], true
	}

	return event, nil, true
}
