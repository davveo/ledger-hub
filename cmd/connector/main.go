package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/config"
	httpserver "github.com/davveo/ledger-hub/internal/iface/http"
	"github.com/davveo/ledger-hub/internal/infrastructure/logger"
	"github.com/davveo/ledger-hub/pkg/client"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "config file")
	flag.Parse()

	cfg := config.MustLoad(*cfgPath)
	zapLog, err := logger.New(cfg.Log)
	if err != nil {
		log.Fatal(err)
	}
	defer zapLog.Sync()

	secrets := map[string]string{}
	for _, cl := range cfg.Gateway.Clients {
		secrets[cl.ClientID] = cl.Secret
	}
	orderCli := client.New(cfg.Connector.LedgerBaseURL, "order", secrets["order"])
	payCli := client.New(cfg.Connector.LedgerBaseURL, "pay", secrets["pay"])

	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "ledger-connector"})
	})
	r.POST("/connector/order/events", handleOrder(orderCli))
	r.POST("/connector/pay/events", handlePay(payCli))
	r.POST("/connector/mq/events", handleMQ(orderCli, payCli))

	if cfg.Connector.MQDir != "" {
		interval := cfg.Connector.MQInterval
		if interval <= 0 {
			interval = 2 * time.Second
		}
		go pollMQ(cfg.Connector.MQDir, interval, orderCli, payCli, zapLog)
	}

	if err := httpserver.Serve(cfg.Connector.Addr, r, cfg.HTTP.ReadTimeout, cfg.HTTP.WriteTimeout, cfg.HTTP.ShutdownTimeout, zapLog); err != nil {
		zapLog.Fatal("connector exit", zap.Error(err))
	}
}

type orderEvent struct {
	Event     string `json:"event"`
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	AssetCode string `json:"asset_code"`
	Amount    string `json:"amount"`
	FreezeID  string `json:"freeze_id"`
}

func handleOrder(cli *client.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var ev orderEvent
		if err := c.ShouldBindJSON(&ev); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": err.Error()})
			return
		}
		raw, err := applyOrder(c.Request.Context(), cli, ev)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": err.Error(), "data": json.RawMessage(raw)})
			return
		}
		c.Data(http.StatusOK, "application/json", raw)
	}
}

func applyOrder(ctx context.Context, cli *client.Client, ev orderEvent) (json.RawMessage, error) {
	if ev.AssetCode == "" {
		ev.AssetCode = "POINT"
	}
	holder := map[string]string{"type": "user", "id": ev.UserID}
	switch ev.Event {
	case "created":
		return cli.Freeze(ctx, map[string]interface{}{
			"source_system": "order", "biz_type": "order_freeze", "biz_no": "order:freeze:" + ev.OrderID,
			"holder": holder, "asset_code": ev.AssetCode, "amount": ev.Amount,
		})
	case "paid":
		body := map[string]interface{}{
			"source_system": "order", "biz_type": "order_capture", "biz_no": "order:capture:" + ev.OrderID,
			"related_biz_no": "order:freeze:" + ev.OrderID, "holder": holder, "asset_code": ev.AssetCode,
		}
		if ev.FreezeID != "" {
			body["freeze_id"] = ev.FreezeID
		}
		return cli.Capture(ctx, body)
	case "cancelled":
		body := map[string]interface{}{
			"source_system": "order", "biz_type": "order_release", "biz_no": "order:release:" + ev.OrderID,
			"related_biz_no": "order:freeze:" + ev.OrderID, "holder": holder, "asset_code": ev.AssetCode,
		}
		if ev.FreezeID != "" {
			body["freeze_id"] = ev.FreezeID
		}
		return cli.Release(ctx, body)
	default:
		return nil, errBadEvent("created/paid/cancelled")
	}
}

type payEvent struct {
	Event        string `json:"event"`
	PayID        string `json:"pay_id"`
	UserID       string `json:"user_id"`
	AssetCode    string `json:"asset_code"`
	Amount       string `json:"amount"`
	RelatedBizNo string `json:"related_biz_no"`
}

func handlePay(cli *client.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var ev payEvent
		if err := c.ShouldBindJSON(&ev); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": err.Error()})
			return
		}
		raw, err := applyPay(c.Request.Context(), cli, ev)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": err.Error(), "data": json.RawMessage(raw)})
			return
		}
		c.Data(http.StatusOK, "application/json", raw)
	}
}

func applyPay(ctx context.Context, cli *client.Client, ev payEvent) (json.RawMessage, error) {
	if ev.AssetCode == "" {
		ev.AssetCode = "BALANCE_CNY"
	}
	holder := map[string]string{"type": "user", "id": ev.UserID}
	switch ev.Event {
	case "paid":
		return cli.Credit(ctx, map[string]interface{}{
			"source_system": "pay", "biz_type": "pay_credit", "biz_no": "pay:credit:" + ev.PayID,
			"holder": holder, "asset_code": ev.AssetCode, "amount": ev.Amount,
		})
	case "refund":
		related := ev.RelatedBizNo
		if related == "" {
			related = "pay:credit:" + ev.PayID
		}
		return cli.Credit(ctx, map[string]interface{}{
			"source_system": "pay", "biz_type": "pay_refund", "biz_no": "pay:refund:" + ev.PayID,
			"related_biz_no": related, "holder": holder, "asset_code": ev.AssetCode, "amount": ev.Amount,
		})
	default:
		return nil, errBadEvent("paid/refund")
	}
}

type mqEvent struct {
	Topic        string `json:"topic"`
	Event        string `json:"event"`
	OrderID      string `json:"order_id"`
	PayID        string `json:"pay_id"`
	UserID       string `json:"user_id"`
	AssetCode    string `json:"asset_code"`
	Amount       string `json:"amount"`
	FreezeID     string `json:"freeze_id"`
	RelatedBizNo string `json:"related_biz_no"`
}

func handleMQ(orderCli, payCli *client.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var ev mqEvent
		if err := c.ShouldBindJSON(&ev); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": err.Error()})
			return
		}
		raw, err := applyMQ(c.Request.Context(), orderCli, payCli, ev)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": err.Error(), "data": json.RawMessage(raw)})
			return
		}
		c.Data(http.StatusOK, "application/json", raw)
	}
}

func applyMQ(ctx context.Context, orderCli, payCli *client.Client, ev mqEvent) (json.RawMessage, error) {
	switch strings.ToLower(ev.Topic) {
	case "order":
		return applyOrder(ctx, orderCli, orderEvent{
			Event: ev.Event, OrderID: ev.OrderID, UserID: ev.UserID,
			AssetCode: ev.AssetCode, Amount: ev.Amount, FreezeID: ev.FreezeID,
		})
	case "pay":
		return applyPay(ctx, payCli, payEvent{
			Event: ev.Event, PayID: ev.PayID, UserID: ev.UserID,
			AssetCode: ev.AssetCode, Amount: ev.Amount, RelatedBizNo: ev.RelatedBizNo,
		})
	default:
		return nil, errBadEvent("topic=order|pay")
	}
}

func pollMQ(dir string, every time.Duration, orderCli, payCli *client.Client, log *zap.Logger) {
	_ = os.MkdirAll(dir, 0o755)
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
			var ev mqEvent
			if json.Unmarshal(b, &ev) != nil {
				continue
			}
			if _, err := applyMQ(context.Background(), orderCli, payCli, ev); err != nil {
				log.Warn("mq apply failed", zap.String("file", e.Name()), zap.Error(err))
				continue
			}
			_ = os.Rename(p, p+".done")
		}
	}
}

type badEvent string

func (e badEvent) Error() string { return string(e) }

func errBadEvent(want string) error { return badEvent("event 需为 " + want) }
