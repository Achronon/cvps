package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

type APIError struct {
	StatusCode int    `json:"-"`
	Message    string `json:"message"`
	Code       string `json:"code,omitempty"`
	// The NestJS backend carries the machine-readable code in `error`
	// (e.g. subscription_required, aup_acceptance_required); `code` is
	// kept for back-compat with older response shapes. The HLM-384
	// scope-enforcement 403s use `code` (insufficient_scope,
	// session_auth_required, api_key_not_permitted).
	ErrorCode string `json:"error,omitempty"`
	// Required carries the missing scope(s) when Code is
	// insufficient_scope (HLM-384 contract).
	Required string `json:"required,omitempty"`
	Details  any    `json:"details,omitempty"`
}

func (e *APIError) UnmarshalJSON(data []byte) error {
	type rawAPIError struct {
		StatusCode int             `json:"statusCode,omitempty"`
		Message    json.RawMessage `json:"message,omitempty"`
		Code       string          `json:"code,omitempty"`
		ErrorCode  string          `json:"error,omitempty"`
		Required   string          `json:"required,omitempty"`
		Details    any             `json:"details,omitempty"`
	}

	var raw rawAPIError
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	e.StatusCode = raw.StatusCode
	e.Message = normalizeAPIErrorMessage(raw.Message)
	e.Code = raw.Code
	e.ErrorCode = raw.ErrorCode
	e.Required = raw.Required
	e.Details = raw.Details
	return nil
}

func normalizeAPIErrorMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var message string
	if err := json.Unmarshal(raw, &message); err == nil {
		return message
	}

	var messages []string
	if err := json.Unmarshal(raw, &messages); err == nil {
		return strings.Join(messages, "; ")
	}

	var values []any
	if err := json.Unmarshal(raw, &values); err == nil {
		parts := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "; ")
	}

	return string(raw)
}

// ErrCode returns the machine-readable error code regardless of which
// field the server used.
func (e *APIError) ErrCode() string {
	if e.Code != "" {
		return e.Code
	}
	return e.ErrorCode
}

func (e *APIError) Error() string {
	code := e.ErrCode()
	// HLM-384 scope-enforcement 403s get actionable rendering so an
	// agent (or human) immediately knows the remediation path.
	switch code {
	case "insufficient_scope":
		required := e.Required
		if required == "" {
			required = "<unknown>"
		}
		return fmt.Sprintf(
			"this token lacks the required scope: %s (mint one with 'cvps token create --name <name> --scope %s' from a session login)",
			required, required,
		)
	case "session_auth_required":
		return "this endpoint requires a session login - minted tokens cannot manage tokens; bootstrap a session with 'cvps login --email <address>' or 'cvps login'"
	case "api_key_not_permitted":
		return "this endpoint does not accept API tokens - use a session login ('cvps login --email <address>' or 'cvps login')"
	}
	switch {
	// Some backend error bodies (e.g. the account-security 403s) carry a
	// code but no message — don't render a dangling "code: ".
	case code != "" && e.Message != "":
		return fmt.Sprintf("%s: %s", code, e.Message)
	case code != "":
		return code
	default:
		return e.Message
	}
}

func IsNotFound(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 404
	}
	return false
}

func IsUnauthorized(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 401
	}
	return false
}

func IsForbidden(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 403
	}
	return false
}
