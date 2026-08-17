package errresp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/internal/domain"
)

func Lang(c *gin.Context) string {
	if c == nil {
		return "zh"
	}
	return domain.LangFrom(c.GetHeader("Lang"), c.GetHeader("X-Lang"), c.GetHeader("Accept-Language"))
}

func Payload(lang string, err error) gin.H {
	de := domain.AsError(err)
	if de == nil {
		return gin.H{"code": 0}
	}
	return gin.H{
		"code":    de.Code,
		"error":   string(de.Key),
		"message": domain.Localize(lang, de),
	}
}

func Write(c *gin.Context, err error) {
	de := domain.AsError(err)
	if de == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0})
		return
	}
	c.JSON(de.HTTPStatus(), Payload(Lang(c), de))
}

func Abort(c *gin.Context, err error) {
	de := domain.AsError(err)
	if de == nil {
		return
	}
	c.AbortWithStatusJSON(de.HTTPStatus(), Payload(Lang(c), de))
}

func WriteWithData(c *gin.Context, err error, data interface{}) {
	de := domain.AsError(err)
	body := Payload(Lang(c), de)
	if data != nil {
		body["data"] = data
	}
	status := http.StatusInternalServerError
	if de != nil {
		status = de.HTTPStatus()
	}
	c.JSON(status, body)
}
