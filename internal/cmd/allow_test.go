package cmd

import "testing"

func TestParseAllowTarget(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTarget string
		wantPort   int
		wantErr    bool
	}{
		{
			name:       "name and port",
			input:      "cortex-worker:8788",
			wantTarget: "cortex-worker",
			wantPort:   8788,
		},
		{
			name:       "id and port with spaces",
			input:      "  sbx-abc123:443  ",
			wantTarget: "sbx-abc123",
			wantPort:   443,
		},
		{
			name:    "missing port",
			input:   "cortex-worker",
			wantErr: true,
		},
		{
			name:    "empty target",
			input:   ":8788",
			wantErr: true,
		},
		{
			name:    "invalid port",
			input:   "cortex-worker:https",
			wantErr: true,
		},
		{
			name:    "out of range port",
			input:   "cortex-worker:70000",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, port, err := parseAllowTarget(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target != tt.wantTarget || port != tt.wantPort {
				t.Fatalf("parseAllowTarget(%q) = %q, %d; want %q, %d", tt.input, target, port, tt.wantTarget, tt.wantPort)
			}
		})
	}
}

func TestAllowCommandRegistered(t *testing.T) {
	if allowCmd.Flags().Lookup("to") == nil {
		t.Fatal("Expected 'cvps allow' to register --to")
	}
	names := map[string]bool{}
	for _, c := range allowCmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"list", "revoke"} {
		if !names[want] {
			t.Errorf("Expected 'cvps allow %s' to be registered", want)
		}
	}
	if allowRevokeCmd.Flags().Lookup("to") == nil {
		t.Fatal("Expected 'cvps allow revoke' to register --to")
	}
}
