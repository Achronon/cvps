package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

type SandboxSecret struct {
	Key  string `json:"key"`
	Name string `json:"name"`
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
	Secrets        []SandboxSecret        `json:"secrets,omitempty"`

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

	// Legacy TenantSecret ids to inject as environment variables. The CLI's
	// repeatable --secret <KEY> flags use SecretKeys below; retain this field
	// for callers that still address secrets by ID.
	SecretIDs []string `json:"secretIds,omitempty"`

	// Known tenant secret keys to resolve server-side under secrets:attach.
	// This avoids requiring the caller to enumerate secrets:read just to attach
	// explicitly requested keys.
	SecretKeys []string `json:"secretKeys,omitempty"`

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

type SandboxAllowRule struct {
	ID                string `json:"id,omitempty"`
	SourceSandboxID   string `json:"sourceSandboxId"`
	SourceSandboxName string `json:"sourceSandboxName,omitempty"`
	TargetSandboxID   string `json:"targetSandboxId"`
	TargetSandboxName string `json:"targetSandboxName,omitempty"`
	Port              int    `json:"port"`
	Protocol          string `json:"protocol"`
	Changed           *bool  `json:"changed,omitempty"`
	CreatedAt         string `json:"createdAt,omitempty"`
}

type CreateSandboxAllowRuleRequest struct {
	TargetSandboxID string `json:"targetSandboxId"`
	Port            int    `json:"port"`
}

type SandboxList struct {
	Data  []Sandbox `json:"data"`
	Total int       `json:"total"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
}

const destructiveGrantHeader = "X-CVPS-Destructive-Grant"

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
	return c.DeleteSandboxWithGrant(ctx, id, "")
}

func (c *Client) DeleteSandboxWithGrant(ctx context.Context, id, grant string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+"/sandboxes/"+id, nil)
	if err != nil {
		return err
	}
	if grant != "" {
		req.Header.Set(destructiveGrantHeader, grant)
	}

	resp, err := c.doAuthenticatedRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return c.checkResponse(resp)
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

// StartSandbox starts a stopped sandbox or retries provisioning for an errored
// sandbox. The backend preserves the sandbox identity and workspace volume.
func (c *Client) StartSandbox(ctx context.Context, id string) (*Sandbox, error) {
	var sandbox Sandbox
	if err := c.Post(ctx, "/sandboxes/"+id+"/start", nil, &sandbox); err != nil {
		return nil, err
	}
	return &sandbox, nil
}

// StopSandbox stops a running or provisioning sandbox without deleting its
// persistent workspace.
func (c *Client) StopSandbox(ctx context.Context, id string) (*Sandbox, error) {
	var sandbox Sandbox
	if err := c.Post(ctx, "/sandboxes/"+id+"/stop", nil, &sandbox); err != nil {
		return nil, err
	}
	return &sandbox, nil
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

func (c *Client) ListSandboxAllowRules(ctx context.Context, sandboxID string) ([]SandboxAllowRule, error) {
	var rules []SandboxAllowRule
	path := fmt.Sprintf(
		"/sandboxes/%s/allow-rules",
		url.PathEscape(sandboxID),
	)
	if err := c.Get(ctx, path, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func (c *Client) AllowSandboxReachability(ctx context.Context, sourceSandboxID, targetSandboxID string, port int) (*SandboxAllowRule, error) {
	var rule SandboxAllowRule
	path := fmt.Sprintf(
		"/sandboxes/%s/allow-rules",
		url.PathEscape(sourceSandboxID),
	)
	req := CreateSandboxAllowRuleRequest{
		TargetSandboxID: targetSandboxID,
		Port:            port,
	}
	if err := c.Post(ctx, path, &req, &rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

func (c *Client) RevokeSandboxReachability(ctx context.Context, sourceSandboxID, targetSandboxID string, port int) (*SandboxAllowRule, error) {
	var rule SandboxAllowRule
	path := fmt.Sprintf(
		"/sandboxes/%s/allow-rules/%s/%d",
		url.PathEscape(sourceSandboxID),
		url.PathEscape(targetSandboxID),
		port,
	)
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.doAuthenticatedRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}
	if err := json.NewDecoder(resp.Body).Decode(&rule); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &rule, nil
}

// AttachSecretToSandbox attaches an existing tenant secret by key to a running
// sandbox and triggers the backend rollout needed for envFrom to be re-read.
func (c *Client) AttachSecretToSandbox(ctx context.Context, sandboxID, key string) (*Sandbox, error) {
	var sandbox Sandbox
	path := fmt.Sprintf(
		"/sandboxes/%s/secrets/%s",
		url.PathEscape(sandboxID),
		url.PathEscape(key),
	)
	if err := c.Post(ctx, path, nil, &sandbox); err != nil {
		return nil, err
	}
	return &sandbox, nil
}

// DetachSecretFromSandbox detaches an existing tenant secret by key from a
// running sandbox and triggers the backend rollout only when the association
// existed.
func (c *Client) DetachSecretFromSandbox(ctx context.Context, sandboxID, key string) (*Sandbox, error) {
	var sandbox Sandbox
	path := fmt.Sprintf(
		"/sandboxes/%s/secrets/%s",
		url.PathEscape(sandboxID),
		url.PathEscape(key),
	)
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.doAuthenticatedRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}
	if err := json.NewDecoder(resp.Body).Decode(&sandbox); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &sandbox, nil
}
