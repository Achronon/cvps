package api

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"
)

// FileOperationResponse is the result of file write/create operations.
type FileOperationResponse struct {
	Success bool   `json:"success"`
	Path    string `json:"path"`
}

// writeFileRequest is the payload of PUT /sandboxes/:id/files/<path>.
type writeFileRequest struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"` // "utf-8" (default) or "base64"
}

// createFileRequest is the payload of POST /sandboxes/:id/files/<path>.
type createFileRequest struct {
	Type string `json:"type"` // "file" or "directory"
}

// filesPath builds the URL path for the files API, escaping each path
// segment while preserving the separators.
func filesPath(sandboxID, remotePath string) string {
	segments := strings.Split(strings.TrimPrefix(remotePath, "/"), "/")
	escaped := make([]string, len(segments))
	for i, s := range segments {
		escaped[i] = url.PathEscape(s)
	}
	return "/sandboxes/" + url.PathEscape(sandboxID) + "/files/" + strings.Join(escaped, "/")
}

// WriteFile writes content to remotePath inside the sandbox via the files
// API (base64-encoded in transit). Parent directories are NOT created by
// the backend; use CreateDirectory first.
func (c *Client) WriteFile(ctx context.Context, sandboxID, remotePath string, content []byte) (*FileOperationResponse, error) {
	var resp FileOperationResponse
	req := &writeFileRequest{
		Content:  base64.StdEncoding.EncodeToString(content),
		Encoding: "base64",
	}
	if err := c.Put(ctx, filesPath(sandboxID, remotePath), req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateDirectory creates a directory (mkdir -p semantics, idempotent)
// inside the sandbox.
func (c *Client) CreateDirectory(ctx context.Context, sandboxID, remotePath string) (*FileOperationResponse, error) {
	var resp FileOperationResponse
	if err := c.Post(ctx, filesPath(sandboxID, remotePath), &createFileRequest{Type: "directory"}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
