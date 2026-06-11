package api

import (
	"strings"
	"testing"
)

// HLM-384 contract: the backend's machine-readable scope 403s render as
// actionable messages.
func TestAPIErrorScopeRendering(t *testing.T) {
	tests := []struct {
		name string
		err  APIError
		want []string
	}{
		{
			name: "insufficient_scope includes the missing scope and remediation",
			err: APIError{
				StatusCode: 403,
				Code:       "insufficient_scope",
				Required:   "sandboxes:write",
				Message:    "API key is missing required scope: sandboxes:write",
			},
			want: []string{"sandboxes:write", "cvps token create --scope sandboxes:write"},
		},
		{
			name: "session_auth_required points at bootstrap login",
			err: APIError{
				StatusCode: 403,
				Code:       "session_auth_required",
			},
			want: []string{"minted tokens cannot manage tokens", "cvps login --email"},
		},
		{
			name: "api_key_not_permitted points at session login",
			err: APIError{
				StatusCode: 403,
				Code:       "api_key_not_permitted",
			},
			want: []string{"does not accept API tokens", "cvps login"},
		},
		{
			name: "other codes keep the generic rendering",
			err: APIError{
				StatusCode: 403,
				ErrorCode:  "subscription_required",
				Message:    "Upgrade required",
			},
			want: []string{"subscription_required: Upgrade required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("Error() = %q, missing %q", got, want)
				}
			}
		})
	}
}
