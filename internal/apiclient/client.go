// Package apiclient is the typed Go SDK for the BastionDeck HTTP API. The
// CLI, TUI and agent-side tooling all talk through it, keeping one contract.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is an API client.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New constructs a client.
func New(baseURL, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token,
		HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// APIError carries the server error envelope.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// envelope mirrors {data:...} / {error:...}.
type envelope struct {
	Data  json.RawMessage `json:"data"`
	Error *APIError       `json:"error"`
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// The custom header doubles as the cookie-CSRF token; native clients send
	// it unconditionally.
	req.Header.Set("X-BDK-CSRF", "1")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	var env envelope
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			if resp.StatusCode >= 400 {
				return &APIError{Status: resp.StatusCode, Code: "bad_response", Message: string(raw)}
			}
			return nil
		}
	}
	if resp.StatusCode >= 400 {
		if env.Error != nil {
			env.Error.Status = resp.StatusCode
			return env.Error
		}
		return &APIError{Status: resp.StatusCode, Code: "http_error", Message: resp.Status}
	}
	if out != nil && len(env.Data) > 0 {
		return json.Unmarshal(env.Data, out)
	}
	return nil
}

// Get issues a GET.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out)
}

// Post issues a POST.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, nil, body, out)
}

// Patch issues a PATCH.
func (c *Client) Patch(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPatch, path, nil, body, out)
}

// Put issues a PUT.
func (c *Client) Put(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPut, path, nil, body, out)
}

// Delete issues a DELETE.
func (c *Client) Delete(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil, out)
}

// RawGet streams a raw response body (used for downloads).
func (c *Client) RawGet(ctx context.Context, path string, query url.Values) (io.ReadCloser, int, error) {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	return resp.Body, resp.StatusCode, nil
}
