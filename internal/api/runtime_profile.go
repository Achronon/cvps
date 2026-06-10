package api

import "context"

// RuntimeProfile is the backend runtime-profile resource (subset the CLI needs).
type RuntimeProfile struct {
	ID               string   `json:"id"`
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Mode             string   `json:"mode,omitempty"` // INTERACTIVE | SERVICE
	Capabilities     []string `json:"capabilities"`
	IsDefault        bool     `json:"isDefault"`
	SelfServeAllowed bool     `json:"selfServeAllowed"`
}

// GetRuntimeProfileBySlug resolves a profile slug (e.g. "cortex") to the full
// profile, including its id for CreateSandboxRequest.RuntimeProfileID.
func (c *Client) GetRuntimeProfileBySlug(ctx context.Context, slug string) (*RuntimeProfile, error) {
	var profile RuntimeProfile
	if err := c.Get(ctx, "/runtime-profiles/slug/"+slug, &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}
