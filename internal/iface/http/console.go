package httpserver

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed console.html
var consoleHTML string

func (s *Server) consolePage(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, consoleHTML)
}
