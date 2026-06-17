package api

import (
	"context"
	"fmt"
)

type SandboxDedicatedIp struct {
	ID string `json:"id"`
	// Null until the coordination claim resolves, so keep it a plain
	// string ("" when absent) rather than failing decode.
	IPAddress string `json:"ipAddress"`
}

// SandboxRuntimeProfile is the profile summary embedded in sandbox responses.
type SandboxRuntimeProfile struct {
	ID           string   `json:"id"`
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Mode         string   `json:"mode,omitempty"` // INTERACTIVE | SERVICE
	Capabilities []string `json:"capabilities"`
}

type Sandbox struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	CPUCores   int    `json:"cpuCores"`
	MemoryGB   int    `json:"memoryGb"`
	StorageGB  int    `json:"storageGb"`
	CreatedAt  string `json:"createdAt"`
	LastActive string `json:"lastActiveAt,omitempty"`

	// HLM-367: Deployment-backed agent-service sandbox (survives node events, exempt
	// from the inactivity reaper; readiness usually gates on in-sandbox model auth).
	ServiceMode    bool                   `json:"serviceMode,omitempty"`
	RuntimeProfile *SandboxRuntimeProfile `json:"runtimeProfile,omitempty"`

	// Dedicated egress IP, when one is assigned (auto-claimed from the
	// plan's included IP or explicitly requested with useDedicatedIp).
	DedicatedIp *SandboxDedicatedIp `json:"dedicatedIp,omitempty"`

	// Connection info (when running)
	SSHHost string `json:"sshHost,omitempty"`
	SSHPort int    `json:"sshPort,omitempty"`
	SSHUser string `json:"sshUser,omitempty"`

	Connectivity struct {
		SSHDirect         bool `json:"sshDirect"`
		SSHProxyRequired  bool `json:"sshProxyRequired"`
		WebsocketTerminal bool `json:"websocketTerminal"`
	} `json:"connectivity"`
}

type CreateSandboxRequest struct {
	Name      string `json:"name"`
	CPUCores  int    `json:"cpuCores,omitempty"`
	MemoryGB  int    `json:"memoryGb,omitempty"`
	StorageGB int    `json:"storageGb,omitempty"`

	// Runtime profile to provision from (resolved from --profile <slug>).
	RuntimeProfileID string `json:"runtimeProfileId,omitempty"`

	// TenantSecret ids to inject as environment variables (resolved from
	// repeatable --secret <KEY> flags; attach is create-time only).
	SecretIDs []string `json:"secretIds,omitempty"`

	// Per-sandbox env overrides (repeatable --env KEY=VALUE flags). Keys
	// must be in the runtime profile's tenantEnvKeys allowlist (HLM-368).
	EnvOverrides map[string]string `json:"envOverrides,omitempty"`

	// Explicitly request a dedicated egress IP. The backend requires
	// AcceptedAup alongside it (and for mail-capable runtimes).
	UseDedicatedIp bool `json:"useDedicatedIp,omitempty"`
	AcceptedAup    bool `json:"acceptedAup,omitempty"`
}

// SandboxLogs is the response of GET /sandboxes/:id/logs.
type SandboxLogs struct {
	PodName string `json:"podName"`
	Logs    string `json:"logs"`
}

type SandboxList struct {
	Data  []Sandbox `json:"data"`
	Total int       `json:"total"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
}

func (c *Client) CreateSandbox(ctx context.Context, req *CreateSandboxRequest) (*Sandbox, error) {
	var sandbox Sandbox
	if err := c.Post(ctx, "/sandboxes", req, &sandbox); err != nil {
		return nil, err
	}
	return &sandbox, nil
}

func (c *Client) ListSandboxes(ctx context.Context, page, limit int) (*SandboxList, error) {
	var list SandboxList
	path := fmt.Sprintf("/sandboxes?page=%d&limit=%d", page, limit)
	if err := c.Get(ctx, path, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

func (c *Client) GetSandbox(ctx context.Context, id string) (*Sandbox, error) {
	var sandbox Sandbox
	if err := c.Get(ctx, "/sandboxes/"+id, &sandbox); err != nil {
		return nil, err
	}
	return &sandbox, nil
}

func (c *Client) GetSandboxStatus(ctx context.Context, id string) (*Sandbox, error) {
	var sandbox Sandbox
	if err := c.Get(ctx, "/sandboxes/"+id+"/status", &sandbox); err != nil {
		return nil, err
	}
	return &sandbox, nil
}

func (c *Client) DeleteSandbox(ctx context.Context, id string) error {
	return c.Delete(ctx, "/sandboxes/"+id)
}

// GetSandboxLogs fetches recent main-container logs (service sandboxes resolve
// controller-generated pod names server-side).
func (c *Client) GetSandboxLogs(ctx context.Context, id string, tailLines int) (*SandboxLogs, error) {
	path := "/sandboxes/" + id + "/logs"
	if tailLines > 0 {
		path = fmt.Sprintf("%s?tailLines=%d", path, tailLines)
	}
	var logs SandboxLogs
	if err := c.Get(ctx, path, &logs); err != nil {
		return nil, err
	}
	return &logs, nil
}

// RestartSandbox bounces the workload (stop + re-provision; service sandboxes
// scale to zero and re-apply their Deployment).
func (c *Client) RestartSandbox(ctx context.Context, id string) (*Sandbox, error) {
	var sandbox Sandbox
	if err := c.Post(ctx, "/sandboxes/"+id+"/restart", nil, &sandbox); err != nil {
		return nil, err
	}
	return &sandbox, nil
}

// ResizeSandboxRequest is the body of PATCH /sandboxes/:id (HLM-432).
type ResizeSandboxRequest struct {
	// StorageGB is the new total workspace size in GB. Grow only — the backend
	// rejects a value at or below the current size (Longhorn cannot shrink).
	StorageGB int `json:"storageGb"`
}

// ResizeSandbox grows a sandbox's workspace storage to newStorageGB. Storage is
// grow-only; Longhorn expands the attached RWO volume online (no pod restart).
func (c *Client) ResizeSandbox(ctx context.Context, id string, newStorageGB int) (*Sandbox, error) {
	var sandbox Sandbox
	req := &ResizeSandboxRequest{StorageGB: newStorageGB}
	if err := c.Patch(ctx, "/sandboxes/"+id, req, &sandbox); err != nil {
		return nil, err
	}
	return &sandbox, nil
}
