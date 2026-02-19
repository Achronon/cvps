package terminal

import "testing"

func TestBuildSocketIOURL(t *testing.T) {
	engineURL, namespace, err := buildSocketIOURL(
		"wss://api.claudevps.com/terminal?sandbox=cmlt6ghp0000101dyq5j3d5xu",
		"token-123",
	)
	if err != nil {
		t.Fatalf("buildSocketIOURL() error = %v, want nil", err)
	}

	if namespace != "/terminal" {
		t.Fatalf("buildSocketIOURL() namespace = %q, want %q", namespace, "/terminal")
	}

	expected := "wss://api.claudevps.com/socket.io/?EIO=4&sandbox=cmlt6ghp0000101dyq5j3d5xu&token=token-123&transport=websocket"
	if engineURL != expected {
		t.Fatalf("buildSocketIOURL() = %q, want %q", engineURL, expected)
	}
}

func TestParseSocketIOEvent(t *testing.T) {
	tests := []struct {
		name      string
		packet    string
		wantEvent string
		wantOK    bool
	}{
		{
			name:      "namespaced event",
			packet:    `2/terminal,["terminal:started",{"sessionId":"abc"}]`,
			wantEvent: "terminal:started",
			wantOK:    true,
		},
		{
			name:      "root namespace event",
			packet:    `2["connected",{"message":"ok"}]`,
			wantEvent: "connected",
			wantOK:    true,
		},
		{
			name:      "non-event packet",
			packet:    `0/terminal,{"sid":"x"}`,
			wantEvent: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, _, ok := parseSocketIOEvent(tt.packet)
			if ok != tt.wantOK {
				t.Fatalf("parseSocketIOEvent() ok = %v, want %v", ok, tt.wantOK)
			}
			if event != tt.wantEvent {
				t.Fatalf("parseSocketIOEvent() event = %q, want %q", event, tt.wantEvent)
			}
		})
	}
}
