package httpserver

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/api"
)

//go:embed console.html
var consoleHTML string

func (s *Server) consolePage(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, consoleHTML)
}

func (s *Server) openapiSpec(c *gin.Context) {
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", api.OpenAPI)
}
