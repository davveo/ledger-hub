package connector

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/pkg/client"
)

type OrderEvent struct {
	Event     string `json:"event"`
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	AssetCode string `json:"asset_code"`
	Amount    string `json:"amount"`
	FreezeID  string `json:"freeze_id"`
}

type PayEvent struct {
	Event        string `json:"event"`
	PayID        string `json:"pay_id"`
	UserID       string `json:"user_id"`
	AssetCode    string `json:"asset_code"`
	Amount       string `json:"amount"`
	RelatedBizNo string `json:"related_biz_no"`
}

type MQEvent struct {
	MessageID     string `json:"message_id"`
	Topic         string `json:"topic"`
	SchemaVersion int    `json:"schema_version"`
	Event         string `json:"event"`
	OrderID       string `json:"order_id"`
	PayID         string `json:"pay_id"`
	UserID        string `json:"user_id"`
	AssetCode     string `json:"asset_code"`
	Amount        string `json:"amount"`
	FreezeID      string `json:"freeze_id"`
	RelatedBizNo  string `json:"related_biz_no"`
}

func ApplyOrder(ctx context.Context, cli *client.Client, ev OrderEvent) (json.RawMessage, error) {
	if ev.AssetCode == "" {
		ev.AssetCode = "POINT"
	}
	holder := client.Holder{Type: "user", ID: ev.UserID}
	switch ev.Event {
	case "created":
		return cli.Freeze(ctx, client.Command{
			SourceSystem: "order", BizType: "order_freeze", BizNo: "order:freeze:" + ev.OrderID,
			Holder: holder, AssetCode: ev.AssetCode, Amount: ev.Amount,
		})
	case "paid":
		cmd := client.Command{
			SourceSystem: "order", BizType: "order_capture", BizNo: "order:capture:" + ev.OrderID,
			RelatedBizNo: "order:freeze:" + ev.OrderID, Holder: holder, AssetCode: ev.AssetCode,
		}
		if ev.FreezeID != "" {
			cmd.FreezeID = ev.FreezeID
		}
		return cli.Capture(ctx, cmd)
	case "cancelled":
		cmd := client.Command{
			SourceSystem: "order", BizType: "order_release", BizNo: "order:release:" + ev.OrderID,
			RelatedBizNo: "order:freeze:" + ev.OrderID, Holder: holder, AssetCode: ev.AssetCode,
		}
		if ev.FreezeID != "" {
			cmd.FreezeID = ev.FreezeID
		}
		return cli.Release(ctx, cmd)
	default:
		return nil, domain.Keyed(domain.CodeUnknownEvent, domain.KeyUnknownEvent)
	}
}

func ApplyPay(ctx context.Context, cli *client.Client, ev PayEvent) (json.RawMessage, error) {
	if ev.AssetCode == "" {
		ev.AssetCode = "BALANCE_CNY"
	}
	holder := client.Holder{Type: "user", ID: ev.UserID}
	switch ev.Event {
	case "paid":
		return cli.Credit(ctx, client.Command{
			SourceSystem: "pay", BizType: "pay_credit", BizNo: "pay:credit:" + ev.PayID,
			Holder: holder, AssetCode: ev.AssetCode, Amount: ev.Amount,
		})
	case "refund":
		related := ev.RelatedBizNo
		if related == "" {
			related = "pay:credit:" + ev.PayID
		}
		return cli.Credit(ctx, client.Command{
			SourceSystem: "pay", BizType: "pay_refund", BizNo: "pay:refund:" + ev.PayID,
			RelatedBizNo: related, Holder: holder, AssetCode: ev.AssetCode, Amount: ev.Amount,
		})
	default:
		return nil, domain.Keyed(domain.CodeUnknownEvent, domain.KeyUnknownEvent)
	}
}

func ApplyMQ(ctx context.Context, orderCli, payCli *client.Client, ev MQEvent) (json.RawMessage, error) {
	switch strings.ToLower(ev.Topic) {
	case "order":
		return ApplyOrder(ctx, orderCli, OrderEvent{
			Event: ev.Event, OrderID: ev.OrderID, UserID: ev.UserID,
			AssetCode: ev.AssetCode, Amount: ev.Amount, FreezeID: ev.FreezeID,
		})
	case "pay":
		return ApplyPay(ctx, payCli, PayEvent{
			Event: ev.Event, PayID: ev.PayID, UserID: ev.UserID,
			AssetCode: ev.AssetCode, Amount: ev.Amount, RelatedBizNo: ev.RelatedBizNo,
		})
	default:
		return nil, domain.Keyed(domain.CodeUnknownEvent, domain.KeyUnknownEvent)
	}
}
