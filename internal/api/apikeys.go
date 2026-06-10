package api

import (
	"context"
	"net/url"
)

// APIKey is the backend ApiKey resource. The full key material is only
// present in the response to CreateApiKey (and is never stored or echoed
// by any other endpoint).
type APIKey struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Key       string   `json:"key,omitempty"` // only on creation
	KeyPrefix string   `json:"keyPrefix"`
	Scopes    []string `json:"scopes"`
	CreatedAt string   `json:"createdAt"`
	ExpiresAt string   `json:"expiresAt,omitempty"`
	LastUsed  string   `json:"lastUsedAt,omitempty"`
}

// CreateAPIKeyRequest is the payload of POST /api-keys. When Scopes is
// omitted the backend defaults to ["sandboxes:read", "sandboxes:write"].
type CreateAPIKeyRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes,omitempty"`
	ExpiresAt string   `json:"expiresAt,omitempty"` // ISO 8601
}

// CreateAPIKey mints a new API key. The returned APIKey.Key is the only
// time the full key material is available.
func (c *Client) CreateAPIKey(ctx context.Context, req *CreateAPIKeyRequest) (*APIKey, error) {
	var key APIKey
	if err := c.Post(ctx, "/api-keys", req, &key); err != nil {
		return nil, err
	}
	return &key, nil
}

// ListAPIKeys lists the user's non-revoked API keys (never includes key
// material).
func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	var keys []APIKey
	if err := c.Get(ctx, "/api-keys", &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// RevokeAPIKey revokes an API key by id. NOTE: the backend silently
// no-ops on unknown ids, so callers should resolve/verify the id against
// ListAPIKeys first if they want not-found feedback.
func (c *Client) RevokeAPIKey(ctx context.Context, id string) error {
	return c.Delete(ctx, "/api-keys/"+url.PathEscape(id))
}
