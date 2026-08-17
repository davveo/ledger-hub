package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/observability"
	"github.com/davveo/ledger-hub/pkg/client"
)

type HTTP struct {
	proc     *Processor
	orderCli *client.Client
	payCli   *client.Client
	ready    func(context.Context) error
}

func NewHTTP(proc *Processor, orderCli, payCli *client.Client, ready func(context.Context) error) *HTTP {
	return &HTTP{proc: proc, orderCli: orderCli, payCli: payCli, ready: ready}
}

func (h *HTTP) Engine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger(), observability.HTTPMetrics())
	observability.RegisterProbes(r, "ledger-connector", h.ready)
	r.GET("/metrics", observability.MetricsHandler())
	r.POST("/connector/order/events", h.handleOrder)
	r.POST("/connector/pay/events", h.handlePay)
	r.POST("/connector/mq/events", h.handleMQ)
	r.GET("/connector/mq/inbox", h.listInbox)
	r.POST("/connector/mq/inbox/:id/replay", h.replayInbox)
	return r
}

func (h *HTTP) handleOrder(c *gin.Context) {
	var ev OrderEvent
	if err := c.ShouldBindJSON(&ev); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": err.Error()})
		return
	}
	raw, err := ApplyOrder(c.Request.Context(), h.orderCli, ev)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": err.Error(), "data": json.RawMessage(raw)})
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *HTTP) handlePay(c *gin.Context) {
	var ev PayEvent
	if err := c.ShouldBindJSON(&ev); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": err.Error()})
		return
	}
	raw, err := ApplyPay(c.Request.Context(), h.payCli, ev)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": err.Error(), "data": json.RawMessage(raw)})
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *HTTP) handleMQ(c *gin.Context) {
	rawBody, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": err.Error()})
		return
	}
	var ev MQEvent
	if err := json.Unmarshal(rawBody, &ev); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": err.Error()})
		return
	}
	if ev.SchemaVersion == 0 {
		ev.SchemaVersion = 1
	}
	id := ev.MessageID
	if id == "" {
		id = ev.Topic + ":" + ev.Event + ":" + ev.OrderID + ev.PayID
	}
	msg, err := h.proc.Ingest(c.Request.Context(), id, ev.Topic, ev.SchemaVersion, rawBody)
	if err != nil && msg == nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": err.Error()})
		return
	}
	status := http.StatusOK
	if msg != nil && msg.Status == "dead" {
		status = http.StatusBadGateway
	}
	c.JSON(status, gin.H{"code": 0, "data": msg})
}

func (h *HTTP) listInbox(c *gin.Context) {
	list, err := h.proc.Inbox().List(c.Request.Context(), c.Query("status"), 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": list})
}

func (h *HTTP) replayInbox(c *gin.Context) {
	msg, err := h.proc.Replay(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": err.Error(), "data": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": msg})
}

func PollFiles(dir string, every time.Duration, proc *Processor, log *zap.Logger) {
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	if every <= 0 {
		every = 2 * time.Second
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for range tick.C {
		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Warn("mq dir read failed", zap.Error(err))
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var ev MQEvent
			if json.Unmarshal(b, &ev) != nil {
				continue
			}
			if ev.SchemaVersion == 0 {
				ev.SchemaVersion = 1
			}
			id := ev.MessageID
			if id == "" {
				id = strings.TrimSuffix(e.Name(), ".json")
			}
			if _, err := proc.Ingest(context.Background(), id, ev.Topic, ev.SchemaVersion, b); err != nil {
				log.Warn("mq apply failed", zap.String("file", e.Name()), zap.Error(err))
				continue
			}
			_ = os.Rename(p, p+".done")
		}
		_, _ = proc.DrainDue(context.Background(), 50)
	}
}
