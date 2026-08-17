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
	for _, want := range []string{"Ledger Hub", "资产", "对账", "风控", "作业", "审计", "过期待释放", "X-Operator", "/api/v1/ledger/console/overview", "modalBackdrop", "promptDialog", "confirmDialog"} {
		if !strings.Contains(body, want) {
			t.Fatalf("console html missing %q", want)
		}
	}
	for _, unwanted := range []string{"window.alert(", "window.confirm(", "window.prompt(", "if (!confirm(", "const raw = prompt("} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("console html still uses native dialog %q", unwanted)
		}
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
