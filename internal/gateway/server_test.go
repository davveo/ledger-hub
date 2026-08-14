package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/internal/config"
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
