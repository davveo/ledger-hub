package observability

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/iface/errresp"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

func RegisterProbes(r gin.IRoutes, service string, ready func(context.Context) error) {
	live := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": service})
	}
	r.GET("/livez", live)
	r.GET("/healthz", live)
	r.GET("/readyz", func(c *gin.Context) {
		if ready == nil {
			errresp.Write(c, domain.Keyed(domain.CodeNotReady, domain.KeyNotReady))
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			errresp.Write(c, domain.Keyed(domain.CodeNotReady, domain.KeyNotReady))
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "status": "ready", "service": service})
	})
}

func ClusterReady(p Pinger) func(context.Context) error {
	return func(ctx context.Context) error {
		if p == nil {
			return errNotReady{"cluster is nil"}
		}
		return p.Ping(ctx)
	}
}

type errNotReady struct{ s string }

func (e errNotReady) Error() string { return e.s }

type CachedCheck struct {
	fn    func(context.Context) error
	ttl   time.Duration
	mu    sync.Mutex
	err   error
	at    time.Time
}

func NewCachedCheck(fn func(context.Context) error, ttl time.Duration) *CachedCheck {
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	return &CachedCheck{fn: fn, ttl: ttl}
}

func (c *CachedCheck) Ready(ctx context.Context) error {
	if c == nil || c.fn == nil {
		return errNotReady{"not configured"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < c.ttl {
		return c.err
	}
	c.err = c.fn(ctx)
	c.at = time.Now()
	return c.err
}

func HTTPReady(url string, timeout time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		if url == "" {
			return errNotReady{"upstream empty"}
		}
		if timeout <= 0 {
			timeout = 2 * time.Second
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode >= 500 {
			return errNotReady{"upstream status " + resp.Status}
		}
		return nil
	}
}
