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

	"github.com/google/uuid"

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
	Error   string          `json:"error,omitempty"`
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
	baseURL    string
	clientID   string
	secret     string
	keyVersion string
	tenantID   string
	requestID  string
	lang       string
	retries    int
	http       *http.Client
}

func New(baseURL, clientID, secret string) *Client {
	return &Client{
		baseURL:    baseURL,
		clientID:   clientID,
		secret:     secret,
		keyVersion: "1",
		retries:    2,
		http:       &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) WithTenant(tenantID string) *Client {
	c.tenantID = tenantID
	return c
}

func (c *Client) WithKeyVersion(version string) *Client {
	if version != "" {
		c.keyVersion = version
	}
	return c
}

func (c *Client) WithTimeout(d time.Duration) *Client {
	if c.http == nil {
		c.http = &http.Client{}
	}
	c.http.Timeout = d
	return c
}

func (c *Client) WithRequestID(id string) *Client {
	c.requestID = id
	return c
}

func (c *Client) WithLang(lang string) *Client {
	c.lang = lang
	return c
}

func (c *Client) WithRetries(n int) *Client {
	c.retries = n
	return c
}

func (c *Client) Credit(ctx context.Context, cmd Command) (json.RawMessage, error) {
	cmd.Command = "Credit"
	return c.do(ctx, http.MethodPost, "/api/v1/ledger/commands/credit", cmd)
}
func (c *Client) Debit(ctx context.Context, cmd Command) (json.RawMessage, error) {
	cmd.Command = "Debit"
	return c.do(ctx, http.MethodPost, "/api/v1/ledger/commands/debit", cmd)
}
func (c *Client) Freeze(ctx context.Context, cmd Command) (json.RawMessage, error) {
	cmd.Command = "Freeze"
	return c.do(ctx, http.MethodPost, "/api/v1/ledger/commands/freeze", cmd)
}
func (c *Client) Capture(ctx context.Context, cmd Command) (json.RawMessage, error) {
	cmd.Command = "Capture"
	return c.do(ctx, http.MethodPost, "/api/v1/ledger/commands/capture", cmd)
}
func (c *Client) Release(ctx context.Context, cmd Command) (json.RawMessage, error) {
	cmd.Command = "Release"
	return c.do(ctx, http.MethodPost, "/api/v1/ledger/commands/release", cmd)
}
func (c *Client) Transfer(ctx context.Context, cmd Command) (json.RawMessage, error) {
	cmd.Command = "Transfer"
	return c.do(ctx, http.MethodPost, "/api/v1/ledger/commands/transfer", cmd)
}
func (c *Client) Exchange(ctx context.Context, cmd Command) (json.RawMessage, error) {
	cmd.Command = "Exchange"
	return c.do(ctx, http.MethodPost, "/api/v1/ledger/commands/exchange", cmd)
}
func (c *Client) Reverse(ctx context.Context, cmd Command) (json.RawMessage, error) {
	cmd.Command = "Reverse"
	return c.do(ctx, http.MethodPost, "/api/v1/ledger/commands/reverse", cmd)
}

func (c *Client) Exec(ctx context.Context, cmd Command) (*CommandResult, error) {
	path := "/api/v1/ledger/commands"
	if cmd.Command != "" {
		if p := toPath(cmd.Command); p != "" {
			path = path + "/" + p
		}
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

func (c *Client) do(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	var last json.RawMessage
	attempts := c.retries + 1
	if attempts < 1 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		raw, status, err := c.roundTrip(ctx, method, path, body)
		if err == nil {
			return raw, nil
		}
		last = raw
		if status != http.StatusBadGateway && status != http.StatusServiceUnavailable {
			return raw, err
		}
		if i+1 < attempts {
			time.Sleep(50 * time.Millisecond)
		} else {
			return raw, err
		}
	}
	return last, fmt.Errorf("exhausted retries")
}

func (c *Client) roundTrip(ctx context.Context, method, path string, body interface{}) (json.RawMessage, int, error) {
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	u, _ := url.Parse(c.baseURL + path)
	ts := sign.Timestamp()
	nonce := uuid.NewString()
	tenant := c.tenantID
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Id", c.clientID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Sign-Version", sign.VersionV2)
	req.Header.Set("X-Key-Version", c.keyVersion)
	req.Header.Set("X-Nonce", nonce)
	if c.requestID != "" {
		req.Header.Set("X-Request-Id", c.requestID)
	}
	if tenant != "" {
		req.Header.Set("X-Tenant-Id", tenant)
	}
	if c.lang != "" {
		req.Header.Set("Lang", c.lang)
		req.Header.Set("Accept-Language", c.lang)
	}
	req.Header.Set("X-Signature", sign.HMACV2(c.secret, c.clientID, method, u.Path, u.RawQuery, tenant, ts, nonce, raw))
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 300 {
		return b, resp.StatusCode, fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
	}
	return b, resp.StatusCode, nil
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
