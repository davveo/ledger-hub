package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/davveo/ledger-hub/pkg/sign"
)

type Client struct {
	baseURL  string
	clientID string
	secret   string
	http     *http.Client
}

func New(baseURL, clientID, secret string) *Client {
	return &Client{
		baseURL:  baseURL,
		clientID: clientID,
		secret:   secret,
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) Credit(ctx context.Context, body map[string]interface{}) (json.RawMessage, error) {
	return c.post(ctx, "/api/v1/ledger/commands/credit", body)
}

func (c *Client) Debit(ctx context.Context, body map[string]interface{}) (json.RawMessage, error) {
	return c.post(ctx, "/api/v1/ledger/commands/debit", body)
}

func (c *Client) Freeze(ctx context.Context, body map[string]interface{}) (json.RawMessage, error) {
	return c.post(ctx, "/api/v1/ledger/commands/freeze", body)
}

func (c *Client) Capture(ctx context.Context, body map[string]interface{}) (json.RawMessage, error) {
	return c.post(ctx, "/api/v1/ledger/commands/capture", body)
}

func (c *Client) Release(ctx context.Context, body map[string]interface{}) (json.RawMessage, error) {
	return c.post(ctx, "/api/v1/ledger/commands/release", body)
}

func (c *Client) Transfer(ctx context.Context, body map[string]interface{}) (json.RawMessage, error) {
	return c.post(ctx, "/api/v1/ledger/commands/transfer", body)
}

func (c *Client) Exchange(ctx context.Context, body map[string]interface{}) (json.RawMessage, error) {
	return c.post(ctx, "/api/v1/ledger/commands/exchange", body)
}

func (c *Client) Reverse(ctx context.Context, body map[string]interface{}) (json.RawMessage, error) {
	return c.post(ctx, "/api/v1/ledger/commands/reverse", body)
}

func (c *Client) post(ctx context.Context, path string, body map[string]interface{}) (json.RawMessage, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	ts := sign.Timestamp()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Id", c.clientID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", sign.HMACSHA256(c.clientID, c.secret, ts, raw))
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return b, fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
	}
	return b, nil
}
