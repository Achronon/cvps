package api

import "testing"

func TestFilesPath(t *testing.T) {
	tests := []struct {
		name      string
		sandboxID string
		remote    string
		want      string
	}{
		{
			name:      "simple path",
			sandboxID: "sbx-123",
			remote:    "/workspace/.codex/auth.json",
			want:      "/sandboxes/sbx-123/files/workspace/.codex/auth.json",
		},
		{
			name:      "segment needing escape",
			sandboxID: "sbx-123",
			remote:    "/workspace/my file.txt",
			want:      "/sandboxes/sbx-123/files/workspace/my%20file.txt",
		},
		{
			name:      "question mark does not become a query",
			sandboxID: "sbx-123",
			remote:    "/workspace/a?b.txt",
			want:      "/sandboxes/sbx-123/files/workspace/a%3Fb.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filesPath(tt.sandboxID, tt.remote); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
