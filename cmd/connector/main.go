package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

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
		if ev.AssetCode == "" {
			ev.AssetCode = "POINT"
		}
		holder := map[string]string{"type": "user", "id": ev.UserID}
		var (
			raw json.RawMessage
			err error
		)
		switch ev.Event {
		case "created":
			raw, err = cli.Freeze(c.Request.Context(), map[string]interface{}{
				"source_system": "order",
				"biz_type":      "order_freeze",
				"biz_no":        "order:freeze:" + ev.OrderID,
				"holder":        holder,
				"asset_code":    ev.AssetCode,
				"amount":        ev.Amount,
			})
		case "paid":
			body := map[string]interface{}{
				"source_system":  "order",
				"biz_type":       "order_capture",
				"biz_no":         "order:capture:" + ev.OrderID,
				"related_biz_no": "order:freeze:" + ev.OrderID,
				"holder":         holder,
				"asset_code":     ev.AssetCode,
			}
			if ev.FreezeID != "" {
				body["freeze_id"] = ev.FreezeID
			}
			raw, err = cli.Capture(c.Request.Context(), body)
		case "cancelled":
			body := map[string]interface{}{
				"source_system":  "order",
				"biz_type":       "order_release",
				"biz_no":         "order:release:" + ev.OrderID,
				"related_biz_no": "order:freeze:" + ev.OrderID,
				"holder":         holder,
				"asset_code":     ev.AssetCode,
			}
			if ev.FreezeID != "" {
				body["freeze_id"] = ev.FreezeID
			}
			raw, err = cli.Release(c.Request.Context(), body)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "event 需为 created/paid/cancelled"})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": err.Error(), "data": json.RawMessage(raw)})
			return
		}
		c.Data(http.StatusOK, "application/json", raw)
	}
}

type payEvent struct {
	Event     string `json:"event"`
	PayID     string `json:"pay_id"`
	UserID    string `json:"user_id"`
	AssetCode string `json:"asset_code"`
	Amount    string `json:"amount"`
}

func handlePay(cli *client.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var ev payEvent
		if err := c.ShouldBindJSON(&ev); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": err.Error()})
			return
		}
		if ev.Event != "paid" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "event 需为 paid"})
			return
		}
		if ev.AssetCode == "" {
			ev.AssetCode = "BALANCE_CNY"
		}
		raw, err := cli.Credit(c.Request.Context(), map[string]interface{}{
			"source_system": "pay",
			"biz_type":      "pay_credit",
			"biz_no":        "pay:credit:" + ev.PayID,
			"holder":        map[string]string{"type": "user", "id": ev.UserID},
			"asset_code":    ev.AssetCode,
			"amount":        ev.Amount,
		})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": err.Error(), "data": json.RawMessage(raw)})
			return
		}
		c.Data(http.StatusOK, "application/json", raw)
	}
}
