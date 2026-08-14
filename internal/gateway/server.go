package gateway

import (
	"crypto/hmac"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/pkg/sign"
)

type Server struct {
	cfg   config.GatewayConfig
	proxy *httputil.ReverseProxy
}

func New(cfg config.GatewayConfig) (*Server, error) {
	u, err := url.Parse(cfg.Upstream)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	return &Server{cfg: cfg, proxy: proxy}, nil
}

func (s *Server) Engine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "ledger-gateway"})
	})
	limiter := newLimiter(s.cfg.RateLimitRPS)
	r.Any("/api/*path", s.auth(), limiter.Handle(), s.audit(), s.forward())
	return r
}

func (s *Server) forward() gin.HandlerFunc {
	return func(c *gin.Context) {
		s.proxy.ServeHTTP(c.Writer, c.Request)
	}
}

func (s *Server) auth() gin.HandlerFunc {
	secrets := map[string]string{}
	for _, cl := range s.cfg.Clients {
		secrets[cl.ClientID] = cl.Secret
	}
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			c.Next()
			return
		}
		clientID := c.GetHeader("X-Client-Id")
		ts := c.GetHeader("X-Timestamp")
		sig := c.GetHeader("X-Signature")
		secret, ok := secrets[clientID]
		if !ok || clientID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "未知 client_id"})
			return
		}
		if secret == "" || sig == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "缺少签名"})
			return
		}
		body, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(restore(body))
		expect := sign.HMACSHA256(clientID, secret, ts, body)
		if !hmac.Equal([]byte(expect), []byte(sig)) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "签名校验失败"})
			return
		}
		c.Set("client_id", clientID)
		var peek struct {
			SourceSystem string `json:"source_system"`
		}
		if json.Unmarshal(body, &peek) == nil && peek.SourceSystem != "" && peek.SourceSystem != clientID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": domain.CodeForbidden, "message": "source_system 必须与 client_id 一致"})
			return
		}
		c.Next()
	}
}

func (s *Server) audit() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func restore(b []byte) *readCloser { return &readCloser{b: b} }

type readCloser struct {
	b []byte
	i int
}

func (r *readCloser) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

func (r *readCloser) Close() error { return nil }

type limiter struct {
	rps   int
	mu    sync.Mutex
	count int
	slot  int64
}

func newLimiter(rps int) *limiter {
	if rps <= 0 {
		rps = 200
	}
	return &limiter{rps: rps}
}

func (l *limiter) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now().Unix()
		l.mu.Lock()
		if l.slot != now {
			l.slot = now
			l.count = 0
		}
		l.count++
		over := l.count > l.rps
		l.mu.Unlock()
		if over {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    domain.CodeRateLimited,
				"message": domain.ErrRateLimited.Message,
			})
			return
		}
		c.Next()
	}
}

