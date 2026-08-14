package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/davveo/ledger-hub/pkg/sign"
)

type Holder struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type AmountAsset struct {
	AssetCode string `json:"asset_code,omitempty"`
	Amount    string `json:"amount,omitempty"`
}

type Fx struct {
	RateID     string `json:"rate_id,omitempty"`
	BaseAsset  string `json:"base_asset,omitempty"`
	QuoteAsset string `json:"quote_asset,omitempty"`
	Rate       string `json:"rate,omitempty"`
	RateSource string `json:"rate_source,omitempty"`
}

type Command struct {
	Command      string                 `json:"command,omitempty"`
	TenantID     string                 `json:"tenant_id,omitempty"`
	SourceSystem string                 `json:"source_system"`
	BizType      string                 `json:"biz_type,omitempty"`
	BizNo        string                 `json:"biz_no"`
	Holder       Holder                 `json:"holder,omitempty"`
	AssetCode    string                 `json:"asset_code,omitempty"`
	Amount       string                 `json:"amount,omitempty"`
	FreezeID     string                 `json:"freeze_id,omitempty"`
	RelatedBizNo string                 `json:"related_biz_no,omitempty"`
	ToHolder     *Holder                `json:"to_holder,omitempty"`
	To           *AmountAsset           `json:"to,omitempty"`
	From         *AmountAsset           `json:"from,omitempty"`
	Fee          *AmountAsset           `json:"fee,omitempty"`
	Fx           *Fx                    `json:"fx,omitempty"`
	Ext          map[string]interface{} `json:"ext,omitempty"`
}

type Envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type CommandResult struct {
	Accepted         bool     `json:"accepted"`
	IdempotentReplay bool     `json:"idempotent_replay"`
	FreezeID         string   `json:"freeze_id,omitempty"`
	JournalID        string   `json:"journal_id,omitempty"`
	EntryIDs         []string `json:"entry_ids"`
	Account          *Account `json:"account,omitempty"`
	ToAccount        *Account `json:"to_account,omitempty"`
}

type Account struct {
	AccountID  string `json:"account_id"`
	TenantID   string `json:"tenant_id,omitempty"`
	HolderType string `json:"holder_type,omitempty"`
	HolderID   string `json:"holder_id,omitempty"`
	AssetCode  string `json:"asset_code,omitempty"`
	Available  string `json:"available"`
	Frozen     string `json:"frozen"`
	Status     string `json:"status,omitempty"`
}

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

func (c *Client) Exec(ctx context.Context, cmd Command) (*CommandResult, error) {
	path := "/api/v1/ledger/commands"
	if cmd.Command != "" {
		path = path + "/" + toPath(cmd.Command)
	}
	raw, err := c.do(ctx, http.MethodPost, path, cmd)
	if err != nil {
		return nil, err
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	var res CommandResult
	if len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, &res)
	}
	return &res, nil
}

func (c *Client) AccountsByHolder(ctx context.Context, holderType, holderID, assetCode string) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("holder_type", holderType)
	q.Set("holder_id", holderID)
	if assetCode != "" {
		q.Set("asset_code", assetCode)
	}
	return c.do(ctx, http.MethodGet, "/api/v1/ledger/accounts?"+q.Encode(), nil)
}

func (c *Client) Entries(ctx context.Context, holderType, holderID, assetCode string, limit, offset int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("holder_type", holderType)
	q.Set("holder_id", holderID)
	if assetCode != "" {
		q.Set("asset_code", assetCode)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	return c.do(ctx, http.MethodGet, "/api/v1/ledger/entries?"+q.Encode(), nil)
}

func (c *Client) Journal(ctx context.Context, journalID string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/api/v1/ledger/journals/"+journalID, nil)
}

func (c *Client) FreezesByHolder(ctx context.Context, holderType, holderID, assetCode, status string) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("holder_type", holderType)
	q.Set("holder_id", holderID)
	if assetCode != "" {
		q.Set("asset_code", assetCode)
	}
	if status != "" {
		q.Set("status", status)
	}
	return c.do(ctx, http.MethodGet, "/api/v1/ledger/freezes?"+q.Encode(), nil)
}

func (c *Client) post(ctx context.Context, path string, body map[string]interface{}) (json.RawMessage, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(raw))
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

func toPath(cmd string) string {
	switch cmd {
	case "Credit":
		return "credit"
	case "Debit":
		return "debit"
	case "Freeze":
		return "freeze"
	case "Capture":
		return "capture"
	case "Release":
		return "release"
	case "Transfer":
		return "transfer"
	case "Exchange":
		return "exchange"
	case "Reverse":
		return "reverse"
	default:
		return ""
	}
}
