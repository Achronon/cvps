package api

import "context"

type TerminalWebSocketInfo struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func (c *Client) GetTerminalWebSocket(ctx context.Context, sandboxID string) (*TerminalWebSocketInfo, error) {
	var info TerminalWebSocketInfo
	if err := c.Post(ctx, "/sandboxes/"+sandboxID+"/terminal", nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
