package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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
	for _, want := range []string{"Ledger Hub", "资产", "对账", "风控", "/api/v1/ledger/console/overview"} {
		if !strings.Contains(body, want) {
			t.Fatalf("console html missing %q", want)
		}
	}
}
