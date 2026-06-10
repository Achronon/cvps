package api

import (
	"encoding/json"
	"testing"
)

func TestAPIError_DecodesBackendErrorField(t *testing.T) {
	// Real backend (NestJS) error body shape.
	body := []byte(`{"statusCode":402,"error":"subscription_required","message":"Active subscription required. Subscribe to create sandboxes."}`)

	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if got := apiErr.ErrCode(); got != "subscription_required" {
		t.Errorf("ErrCode() = %q, want subscription_required", got)
	}
	if want := "subscription_required: Active subscription required. Subscribe to create sandboxes."; apiErr.Error() != want {
		t.Errorf("Error() = %q, want %q", apiErr.Error(), want)
	}
}

func TestAPIError_CodeFieldTakesPrecedence(t *testing.T) {
	apiErr := APIError{Code: "legacy_code", ErrorCode: "new_code", Message: "m"}
	if got := apiErr.ErrCode(); got != "legacy_code" {
		t.Errorf("ErrCode() = %q, want legacy_code", got)
	}
}

func TestAPIError_NoCodeFallsBackToMessage(t *testing.T) {
	apiErr := APIError{Message: "plain message"}
	if apiErr.Error() != "plain message" {
		t.Errorf("Error() = %q, want plain message", apiErr.Error())
	}
}
