package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/domain"
)

func TestErrorEnvelopeLang(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{tenant: "t_default", query: application.NewQueryService(nil, nil)}
	r := s.Engine()

	zh := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ledger/entries?from=nope", nil)
	r.ServeHTTP(zh, req)
	if zh.Code != http.StatusBadRequest {
		t.Fatalf("zh status %d %s", zh.Code, zh.Body.String())
	}
	var env struct {
		Code    int    `json:"code"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(zh.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != domain.CodeTimeFromInvalid || env.Error != string(domain.KeyTimeFromInvalid) {
		t.Fatalf("zh envelope %+v", env)
	}
	if env.Message != "from 需为 RFC3339" {
		t.Fatalf("zh message %s", env.Message)
	}

	en := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/ledger/entries?from=nope", nil)
	req2.Header.Set("Lang", "en")
	r.ServeHTTP(en, req2)
	if err := json.Unmarshal(en.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error != string(domain.KeyTimeFromInvalid) || !strings.Contains(env.Message, "RFC3339") {
		t.Fatalf("en envelope %+v", env)
	}
	if env.Message != "from must be RFC3339" {
		t.Fatalf("en message %s", env.Message)
	}

	acc := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/ledger/entries?from=nope", nil)
	req3.Header.Set("Accept-Language", "en-US,en;q=0.8")
	r.ServeHTTP(acc, req3)
	if err := json.Unmarshal(acc.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Message != "from must be RFC3339" {
		t.Fatalf("accept-language %s", env.Message)
	}
}

func TestAmountErrorCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{tenant: "t_default"}
	r := s.Engine()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/commands/credit", strings.NewReader(`{"source_system":"campaign","biz_no":"x","amount":"1.5","holder":{"type":"user","id":"u1"},"asset_code":"POINT"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Lang", "en")
	r.ServeHTTP(w, req)
	var env struct {
		Code    int    `json:"code"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if env.Code != domain.CodeAmountNotInteger || env.Error != string(domain.KeyAmountNotInteger) {
		t.Fatalf("amount envelope %+v body=%s", env, w.Body.String())
	}
	if env.Message != "amount must be a min-unit integer" {
		t.Fatalf("en amount message %s", env.Message)
	}
}
