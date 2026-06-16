package apiclient

import (
	"context"
	"net/url"
)

// Ping verifies connectivity and credentials by reading the instance root.
func (c *apiClient) Ping(ctx context.Context) error {
	q := url.Values{"tree": {"mode"}}
	return c.getJSON(ctx, "/api/json", q, &struct {
		Mode string `json:"mode"`
	}{})
}

// WhoAmI returns the authenticated Jenkins user.
func (c *apiClient) WhoAmI(ctx context.Context) (*User, error) {
	q := url.Values{"tree": {"id,fullName"}}
	var raw struct {
		ID       string `json:"id"`
		FullName string `json:"fullName"`
	}
	if err := c.getJSON(ctx, "/me/api/json", q, &raw); err != nil {
		return nil, err
	}
	return &User{ID: raw.ID, FullName: raw.FullName}, nil
}
