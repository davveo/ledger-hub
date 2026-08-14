package gateway

import (
	"bytes"
	"crypto/hmac"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
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
	audit domain.AuditRepository
}

func New(cfg config.GatewayConfig) (*Server, error) {
	u, err := url.Parse(cfg.Upstream)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	return &Server{cfg: cfg, proxy: proxy}, nil
}

func (s *Server) WithAudit(a domain.AuditRepository) *Server {
	s.audit = a
	return s
}

func (s *Server) Engine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "ledger-gateway"})
	})
	lim := newClientLimiter(s.cfg)
	auth := s.auth()
	audit := s.auditMW()
	fwd := s.forward()
	r.Any("/console", auth, audit, fwd)
	r.Any("/console/*path", auth, audit, fwd)
	r.Any("/api/*path", auth, lim.Handle(), audit, fwd)
	return r
}

func (s *Server) forward() gin.HandlerFunc {
	return func(c *gin.Context) {
		s.proxy.ServeHTTP(c.Writer, c.Request)
	}
}

func (s *Server) maxSkew() time.Duration {
	if s.cfg.MaxSkewSeconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(s.cfg.MaxSkewSeconds) * time.Second
}

func (s *Server) auth() gin.HandlerFunc {
	secrets := map[string]string{}
	for _, cl := range s.cfg.Clients {
		secrets[cl.ClientID] = cl.Secret
	}
	return func(c *gin.Context) {
		if s.cfg.ConsoleToken != "" {
			tok := c.GetHeader("X-Console-Token")
			if tok == "" {
				tok = c.Query("console_token")
			}
			if tok != "" && hmac.Equal([]byte(tok), []byte(s.cfg.ConsoleToken)) {
				c.Set("client_id", "console")
				c.Next()
				return
			}
		}
		clientID := c.GetHeader("X-Client-Id")
		ts := c.GetHeader("X-Timestamp")
		sig := c.GetHeader("X-Signature")
		secret, ok := secrets[clientID]
		if !ok || clientID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "未知 client_id"})
			return
		}
		if secret == "" || sig == "" || ts == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "缺少签名"})
			return
		}
		unix, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "时间戳无效"})
			return
		}
		skew := time.Since(time.Unix(unix, 0))
		if skew < 0 {
			skew = -skew
		}
		if skew > s.maxSkew() {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "时间戳超出允许窗口"})
			return
		}
		body, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		expect := sign.HMACSHA256(clientID, secret, ts, body)
		if !hmac.Equal([]byte(expect), []byte(sig)) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "签名校验失败"})
			return
		}
		c.Set("client_id", clientID)
		if len(body) > 0 {
			var peek struct {
				SourceSystem string `json:"source_system"`
			}
			if json.Unmarshal(body, &peek) == nil && peek.SourceSystem != "" && peek.SourceSystem != clientID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": domain.CodeForbidden, "message": "source_system 必须与 client_id 一致"})
				return
			}
		}
		c.Next()
	}
}

func (s *Server) auditMW() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if s.audit == nil {
			return
		}
		clientID, _ := c.Get("client_id")
		cid, _ := clientID.(string)
		_ = s.audit.Create(c.Request.Context(), &domain.GatewayAudit{
			ClientID:   cid,
			Method:     c.Request.Method,
			Path:       c.Request.URL.Path,
			Status:     c.Writer.Status(),
			RemoteAddr: c.ClientIP(),
			RequestID:  c.GetHeader("X-Request-Id"),
			CreatedAt:  time.Now().UTC(),
		})
	}
}

type clientLimiter struct {
	global *slotLimiter
	per    map[string]*slotLimiter
	mu     sync.Mutex
}

func newClientLimiter(cfg config.GatewayConfig) *clientLimiter {
	cl := &clientLimiter{
		global: newSlotLimiter(cfg.RateLimitRPS),
		per:    map[string]*slotLimiter{},
	}
	for _, c := range cfg.Clients {
		if c.RateLimitRPS > 0 {
			cl.per[c.ClientID] = newSlotLimiter(c.RateLimitRPS)
		}
	}
	return cl
}

func (l *clientLimiter) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		cid, _ := c.Get("client_id")
		id, _ := cid.(string)
		lim := l.global
		l.mu.Lock()
		if id != "" {
			if p := l.per[id]; p != nil {
				lim = p
			}
		}
		l.mu.Unlock()
		if lim.over() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    domain.CodeRateLimited,
				"message": domain.ErrRateLimited.Message,
			})
			return
		}
		c.Next()
	}
}

type slotLimiter struct {
	rps   int
	mu    sync.Mutex
	count int
	slot  int64
}

func newSlotLimiter(rps int) *slotLimiter {
	if rps <= 0 {
		rps = 200
	}
	return &slotLimiter{rps: rps}
}

func (l *slotLimiter) over() bool {
	now := time.Now().Unix()
	l.mu.Lock()
	if l.slot != now {
		l.slot = now
		l.count = 0
	}
	l.count++
	over := l.count > l.rps
	l.mu.Unlock()
	return over
}
