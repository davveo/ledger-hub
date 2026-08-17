package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/internal/application"
)

type okPing struct{}

func (okPing) Ping(context.Context) error { return nil }

func TestLivezReadyz(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{tenant: "t_default"}
	r := s.Engine()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("livez %d", w.Code)
	}
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w2.Code != http.StatusOK {
		t.Fatalf("healthz %d", w2.Code)
	}
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w3.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz nil cluster want 503 got %d", w3.Code)
	}

	s2 := &Server{tenant: "t_default"}
	s2.WithCluster(okPing{})
	r2 := s2.Engine()
	w4 := httptest.NewRecorder()
	r2.ServeHTTP(w4, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w4.Code != http.StatusOK {
		t.Fatalf("readyz ping ok want 200 got %d %s", w4.Code, w4.Body.String())
	}
}

func TestReconcileEnqueue202(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{tenant: "t_default", recon: application.NewReconcileService(nil, nil, nil, application.NewMemoryReconcile())}
	r := s.Engine()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/reconcile/jobs", strings.NewReader(`{"date":"2026-08-16","source_system":"order","asset_code":"POINT"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202 got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"status":"queued"`) {
		t.Fatalf("queued body %s", w.Body.String())
	}
}

func TestConfigRevisionOnReload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	revs := application.NewMemoryConfigRev()
	s := &Server{
		tenant: "t_default",
		reload: func() error { return nil },
		revs:   revs,
		acl:    application.NewACL(nil),
	}
	r := s.Engine()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/ops/reload", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Operator", "ops_test")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reload %d %s", w.Code, w.Body.String())
	}
	list, err := revs.List(context.Background(), 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("revisions %v %v", list, err)
	}
	if list[0].Operator != "ops_test" || list[0].Version != 1 {
		t.Fatalf("rev %+v", list[0])
	}
}
