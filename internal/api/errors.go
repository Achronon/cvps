package api

import "fmt"

type APIError struct {
	StatusCode int    `json:"-"`
	Message    string `json:"message"`
	Code       string `json:"code,omitempty"`
	// The NestJS backend carries the machine-readable code in `error`
	// (e.g. subscription_required, aup_acceptance_required); `code` is
	// kept for back-compat with older response shapes.
	ErrorCode string `json:"error,omitempty"`
	Details   any    `json:"details,omitempty"`
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
	if code := e.ErrCode(); code != "" {
		return fmt.Sprintf("%s: %s", code, e.Message)
	}
	return e.Message
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
