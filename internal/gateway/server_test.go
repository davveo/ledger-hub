package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/pkg/sign"
)

func TestGatewayAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer up.Close()
	gw, err := New(config.GatewayConfig{
		Upstream:       up.URL,
		RateLimitRPS:   50,
		MaxSkewSeconds: 300,
		ConsoleToken:   "tok",
		Clients:        []config.ClientAuth{{ClientID: "wallet", Secret: "s"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(gw.Engine())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/ledger/accounts")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned GET want 401 got %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/ledger/accounts", nil)
	req.Header.Set("X-Console-Token", "tok")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("console token want 200 got %d", resp.StatusCode)
	}

	ts := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/v1/ledger/accounts", nil)
	req.Header.Set("X-Client-Id", "wallet")
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", sign.HMACSHA256("wallet", "s", ts, nil))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("stale ts want 401 got %d", resp.StatusCode)
	}

	ts = sign.Timestamp()
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/v1/ledger/accounts", nil)
	req.Header.Set("X-Client-Id", "wallet")
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", sign.HMACSHA256("wallet", "s", ts, nil))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signed GET want 200 got %d body=%s", resp.StatusCode, body)
	}
}

func doGW(t *testing.T, srv *httptest.Server, req *http.Request) (int, []byte) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, body
}

func jsonCode(body []byte) int {
	var env struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(body, &env)
	return env.Code
}

func TestGatewaySignV2NonceReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tenant-Id") != "t_default" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":40001,"message":"missing tenant"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer up.Close()
	gw, err := New(config.GatewayConfig{
		Upstream:       up.URL,
		RateLimitRPS:   50,
		MaxSkewSeconds: 300,
		DefaultTenant:  "t_default",
		Clients:        []config.ClientAuth{{ClientID: "wallet", Secret: "s", KeyVersion: "1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(gw.Engine())
	defer srv.Close()

	path := "/api/v1/ledger/accounts"
	ts := sign.Timestamp()
	nonce := "n-once-1"
	sig := sign.HMACV2("s", "wallet", http.MethodGet, path, "", "t_default", ts, nonce, nil)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	req.Header.Set("X-Client-Id", "wallet")
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Sign-Version", sign.VersionV2)
	req.Header.Set("X-Key-Version", "1")
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Tenant-Id", "t_default")
	req.Header.Set("X-Signature", sig)
	code, body := doGW(t, srv, req)
	if code != http.StatusOK {
		t.Fatalf("v2 want 200 got %d body=%s", code, body)
	}

	req2, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	req2.Header.Set("X-Client-Id", "wallet")
	req2.Header.Set("X-Timestamp", ts)
	req2.Header.Set("X-Sign-Version", sign.VersionV2)
	req2.Header.Set("X-Key-Version", "1")
	req2.Header.Set("X-Nonce", nonce)
	req2.Header.Set("X-Tenant-Id", "t_default")
	req2.Header.Set("X-Signature", sig)
	code, body = doGW(t, srv, req2)
	if code != http.StatusUnauthorized || jsonCode(body) != domain.CodeReplay {
		t.Fatalf("replay want 401/40102 got %d body=%s", code, body)
	}
}

func TestGatewayRejectConsoleQueryToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer up.Close()
	gw, err := New(config.GatewayConfig{
		Upstream:     up.URL,
		ConsoleToken: "tok",
		Clients:      []config.ClientAuth{{ClientID: "wallet", Secret: "s"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(gw.Engine())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/console?console_token=tok", nil)
	req.Header.Set("X-Console-Token", "tok")
	code, body := doGW(t, srv, req)
	if code != http.StatusUnauthorized {
		t.Fatalf("query console_token want 401 got %d body=%s", code, body)
	}
}

func TestGatewayTenantUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer up.Close()
	gw, err := New(config.GatewayConfig{
		Upstream:       up.URL,
		MaxSkewSeconds: 300,
		DefaultTenant:  "t_default",
		Clients:        []config.ClientAuth{{ClientID: "wallet", Secret: "s", Tenants: []string{"t_default"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(gw.Engine())
	defer srv.Close()

	ts := sign.Timestamp()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/ledger/accounts", nil)
	req.Header.Set("X-Client-Id", "wallet")
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", sign.HMACSHA256("wallet", "s", ts, nil))
	req.Header.Set("X-Tenant-Id", "t_other")
	code, body := doGW(t, srv, req)
	if code != http.StatusForbidden || jsonCode(body) != domain.CodeTenantNotAllowed {
		t.Fatalf("foreign tenant want 403/40301 got %d body=%s", code, body)
	}
}

