package api

import "fmt"

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
