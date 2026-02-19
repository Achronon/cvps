package api

import (
	"context"
	"fmt"
)

type Sandbox struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	CPUCores   int    `json:"cpuCores"`
	MemoryGB   int    `json:"memoryGb"`
	StorageGB  int    `json:"storageGb"`
	CreatedAt  string `json:"createdAt"`
	LastActive string `json:"lastActiveAt,omitempty"`

	// Connection info (when running)
	SSHHost string `json:"sshHost,omitempty"`
	SSHPort int    `json:"sshPort,omitempty"`
	SSHUser string `json:"sshUser,omitempty"`
}

type CreateSandboxRequest struct {
	Name      string `json:"name"`
	CPUCores  int    `json:"cpuCores,omitempty"`
	MemoryGB  int    `json:"memoryGb,omitempty"`
	StorageGB int    `json:"storageGb,omitempty"`
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
