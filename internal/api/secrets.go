package api

import (
	"context"
	"fmt"
	"net/url"
)

// Secret is the backend TenantSecret resource. Values are encrypted at rest
// and never returned by the API after creation.
type Secret struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Key           string `json:"key"`
	Description   string `json:"description,omitempty"`
	Category      string `json:"category,omitempty"`
	LastRotatedAt string `json:"lastRotatedAt,omitempty"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// SecretList is the paginated response of GET /secrets.
type SecretList struct {
	Data  []Secret `json:"data"`
	Total int      `json:"total"`
	Page  int      `json:"page"`
	Limit int      `json:"limit"`
}

// CreateSecretRequest is the payload of POST /secrets. Value is only sent,
// never echoed back in any subsequent response.
type CreateSecretRequest struct {
	Name        string `json:"name"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

// CreateSecret creates a tenant secret.
func (c *Client) CreateSecret(ctx context.Context, req *CreateSecretRequest) (*Secret, error) {
	var secret Secret
	if err := c.Post(ctx, "/secrets", req, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

// ListSecrets fetches one page of the tenant's secrets (values are never
// included).
func (c *Client) ListSecrets(ctx context.Context, page, limit int) (*SecretList, error) {
	var list SecretList
	path := fmt.Sprintf("/secrets?page=%d&limit=%d", page, limit)
	if err := c.Get(ctx, path, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// DeleteSecret permanently deletes a secret (detaching it from any sandboxes).
func (c *Client) DeleteSecret(ctx context.Context, id string) error {
	return c.Delete(ctx, "/secrets/"+url.PathEscape(id))
}

// ListAllSecrets pages through GET /secrets until the tenant's full secret
// set is collected. The backend may clamp the requested page size, so
// termination is driven by the reported total / page emptiness rather than
// the requested limit.
func (c *Client) ListAllSecrets(ctx context.Context) ([]Secret, error) {
	const requestedPageSize = 100

	var all []Secret
	for page := 1; ; page++ {
		list, err := c.ListSecrets(ctx, page, requestedPageSize)
		if err != nil {
			return nil, err
		}
		if len(list.Data) == 0 {
			return all, nil
		}
		all = append(all, list.Data...)
		if len(all) >= list.Total {
			return all, nil
		}
	}
}

// FindSecretByKey resolves a secret env key (e.g. TELEGRAM_BOT_TOKEN) to the
// full secret. Returns nil (no error) when the tenant has no secret with
// that key.
func (c *Client) FindSecretByKey(ctx context.Context, key string) (*Secret, error) {
	secrets, err := c.ListAllSecrets(ctx)
	if err != nil {
		return nil, err
	}
	for i := range secrets {
		if secrets[i].Key == key {
			return &secrets[i], nil
		}
	}
	return nil, nil
}
