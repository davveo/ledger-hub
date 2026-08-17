package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/internal/application"
)

func TestConsolePage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{}
	r := gin.New()
	r.GET("/console", s.consolePage)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Ledger Hub", "首页", "资产", "对账", "风控", "作业", "审计", "过期待释放", "X-Operator", "/api/v1/ledger/console/overview", "modalBackdrop", "promptDialog", "confirmDialog", "downloadFile", "/ops/sagas", "retrySaga", "compensateSaga", "assignDiff", "queued-reconcile", "duration_ms", "instance_id", "config/revisions"} {
		if !strings.Contains(body, want) {
			t.Fatalf("console html missing %q", want)
		}
	}
	for _, unwanted := range []string{"window.alert(", "window.confirm(", "window.prompt(", "if (!confirm(", "const raw = prompt(", "?console_token="} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("console html still uses native dialog %q", unwanted)
		}
	}
}

func TestRootRedirectsToConsole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := (&Server{}).Engine()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("status %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/console" {
		t.Fatalf("location %q", loc)
	}
}

func TestExpirePreviewAsOf(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{jobs: application.NewJobs(nil, nil, nil, nil, "t_default")}
	r := s.Engine()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ledger/ops/jobs/expire/preview?as_of=nope", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad as_of status %d body=%s", w.Code, w.Body.String())
	}
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/ledger/ops/jobs/expire/preview?as_of=2027-01-01", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("preview status %d body=%s", w2.Code, w2.Body.String())
	}
}

func TestParseAmountRejectsNonInteger(t *testing.T) {
	n, err := parseAmount("")
	if err != nil || n != 0 {
		t.Fatalf("empty want 0, got %d err=%v", n, err)
	}
	n, err = parseAmount("1049900")
	if err != nil || n != 1049900 {
		t.Fatalf("int want 1049900 got %d err=%v", n, err)
	}
	for _, raw := range []string{"10.5", "1e3", "1E2", "+10", "abc"} {
		if _, err := parseAmount(raw); err == nil {
			t.Fatalf("%q should be invalid", raw)
		}
	}
}

func TestParseTimeRangeRejectsIllegal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mustFail := func(raw string) {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/entries?"+raw, nil)
		if _, _, err := parseTimeRange(c); err == nil {
			t.Fatalf("want error for %s", raw)
		}
	}
	mustFail("from=not-rfc3339")
	mustFail("to=yesterday")
	mustFail("from=2026-08-17T12:00:00Z&to=2026-08-17T11:00:00Z")
	mustFail("from=2024-01-01T00:00:00Z&to=2026-01-02T00:00:00Z")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/entries?from=2026-08-01T00:00:00Z&to=2026-08-02T00:00:00Z", nil)
	from, to, err := parseTimeRange(c)
	if err != nil || from == nil || to == nil {
		t.Fatalf("valid range err=%v", err)
	}
}

func TestHTTPInvalidAmountAndTime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{tenant: "t_default"}
	r := s.Engine()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/commands/credit", strings.NewReader(
		`{"source_system":"campaign","biz_no":"campaign:bad-amt","holder":{"type":"user","id":"u_alice"},"asset_code":"POINT","amount":"10.5"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid amount want 400 got %d body=%s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/ledger/entries?holder_id=u_alice&from=not-a-time", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("invalid from want 400 got %d body=%s", w2.Code, w2.Body.String())
	}

	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/ledger/entries?holder_id=u_alice&from=2026-08-17T12:00:00Z&to=2026-08-17T11:00:00Z", nil)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("from>=to want 400 got %d body=%s", w3.Code, w3.Body.String())
	}
}
