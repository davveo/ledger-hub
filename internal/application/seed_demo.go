package application

import (
	"context"
	"fmt"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

// SeedDemo 写入一套可重复执行的演示账，全部走记账命令，余额 / 冻结 / 流水自洽。
// 覆盖运营台：租户、资产、多币种账户、流水、冻结、汇率、冲正、对账差异、限额告警、未完成 Saga。
func SeedDemo(ctx context.Context, books *Bookkeeping, assets *AssetService, accounts *AccountService, tenants *TenantService, fx *FxService, recon *ReconcileService, sagas domain.SagaRepository, tenantID string) error {
	if books == nil || assets == nil || tenantID == "" {
		return nil
	}
	for _, tid := range []string{tenantID, "t_mall", "t_legacy", "t_game"} {
		if err := SeedAssets(ctx, assets, tid); err != nil {
			return err
		}
	}
	if tenants != nil {
		for _, t := range []*domain.Tenant{
			{TenantID: tenantID, Name: "默认租户", Status: "active"},
			{TenantID: "t_mall", Name: "商城租户", Status: "active"},
			{TenantID: "t_game", Name: "游戏租户", Status: "active"},
			{TenantID: "t_legacy", Name: "历史租户", Status: "disabled"},
		} {
			if err := tenants.Save(ctx, t); err != nil {
				return err
			}
		}
	}
	if fx != nil {
		now := time.Now().UTC()
		feedAt := now.Add(-2 * time.Hour)
		rates := []*domain.FxRate{
			{RateID: "fxr_demo_cny_usd", TenantID: tenantID, BaseAsset: "BALANCE_CNY", QuoteAsset: "BALANCE_USD", Rate: "0.14000000", RateSource: "manual", CreatedBy: "demo", QuotedAt: now},
			{RateID: "fxr_demo_usd_cny", TenantID: tenantID, BaseAsset: "BALANCE_USD", QuoteAsset: "BALANCE_CNY", Rate: "7.14285714", RateSource: "manual", CreatedBy: "demo", QuotedAt: now},
			{RateID: "fxr_demo_cny_hkd", TenantID: tenantID, BaseAsset: "BALANCE_CNY", QuoteAsset: "BALANCE_HKD", Rate: "1.08000000", RateSource: "feed", CreatedBy: "demo", QuotedAt: feedAt},
			{RateID: "fxr_demo_hkd_cny", TenantID: tenantID, BaseAsset: "BALANCE_HKD", QuoteAsset: "BALANCE_CNY", Rate: "0.92592593", RateSource: "feed", CreatedBy: "demo", QuotedAt: feedAt},
			{RateID: "fxr_demo_usd_hkd", TenantID: tenantID, BaseAsset: "BALANCE_USD", QuoteAsset: "BALANCE_HKD", Rate: "7.80000000", RateSource: "manual", CreatedBy: "demo", QuotedAt: now.Add(-30 * time.Minute)},
		}
		for _, r := range rates {
			if err := fx.Save(ctx, r); err != nil {
				return err
			}
		}
	}
	if _, err := assets.SetStatus(ctx, tenantID, "GROWTH", domain.AssetDisabled); err != nil {
		return err
	}
	for _, tid := range []string{"t_mall", "t_legacy", "t_game"} {
		if _, err := assets.SetStatus(ctx, tid, "GROWTH", domain.AssetDisabled); err != nil {
			return err
		}
	}

	alice := domain.Holder{Type: domain.HolderUser, ID: "u_alice"}
	bob := domain.Holder{Type: domain.HolderUser, ID: "u_bob"}
	carol := domain.Holder{Type: domain.HolderUser, ID: "u_carol"}
	dormant := domain.Holder{Type: domain.HolderUser, ID: "u_dormant"}
	dave := domain.Holder{Type: domain.HolderUser, ID: "u_dave"}
	eve := domain.Holder{Type: domain.HolderUser, ID: "u_eve"}
	vip := domain.Holder{Type: domain.HolderUser, ID: "u_vip"}
	frank := domain.Holder{Type: domain.HolderUser, ID: "u_frank"}
	closed := domain.Holder{Type: domain.HolderUser, ID: "u_closed"}
	gamer := domain.Holder{Type: domain.HolderUser, ID: "u_gamer"}
	shop := domain.Holder{Type: domain.HolderMerchant, ID: "m_shop"}
	cafe := domain.Holder{Type: domain.HolderMerchant, ID: "m_cafe"}
	mallShop := domain.Holder{Type: domain.HolderMerchant, ID: "m_mallshop"}
	buyer := domain.Holder{Type: domain.HolderUser, ID: "u_buyer"}
	exp := time.Now().UTC().Add(72 * time.Hour)
	soon := time.Now().UTC().Add(6 * time.Hour)
	past := time.Now().UTC().Add(-2 * time.Hour)

	grants := []domain.CommandRequest{
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:alice:point", Holder: alice, AssetCode: "POINT", Amount: 10000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:bob:point", Holder: bob, AssetCode: "POINT", Amount: 3000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:carol:point", Holder: carol, AssetCode: "POINT", Amount: 200},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:dormant:point", Holder: dormant, AssetCode: "POINT", Amount: 120},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:dave:point", Holder: dave, AssetCode: "POINT", Amount: 5000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:dave:bonus", Holder: dave, AssetCode: "POINT", Amount: 80},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:dave:checkin", Holder: dave, AssetCode: "POINT", Amount: 200},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:dave:task", Holder: dave, AssetCode: "POINT", Amount: 150},
		{Command: domain.CmdCredit, TenantID: "t_mall", SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:mall:point", Holder: buyer, AssetCode: "POINT", Amount: 2000},
		{Command: domain.CmdCredit, TenantID: "t_mall", SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:mall:coupon", Holder: buyer, AssetCode: "POINT", Amount: 500},
		{Command: domain.CmdCredit, TenantID: "t_game", SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:gamer:point", Holder: gamer, AssetCode: "POINT", Amount: 12000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "pay", BizType: "topup", BizNo: "pay:demo:alice:cny", Holder: alice, AssetCode: "BALANCE_CNY", Amount: 1280000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "pay", BizType: "topup", BizNo: "pay:demo:bob:cny", Holder: bob, AssetCode: "BALANCE_CNY", Amount: 200000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "pay", BizType: "topup", BizNo: "pay:demo:bob:usd", Holder: bob, AssetCode: "BALANCE_USD", Amount: 5000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "pay", BizType: "topup", BizNo: "pay:demo:eve:cny", Holder: eve, AssetCode: "BALANCE_CNY", Amount: 200000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "wallet", BizType: "grant", BizNo: "wallet:demo:alice:coin", Holder: alice, AssetCode: "COIN", Amount: 8000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "wallet", BizType: "grant", BizNo: "wallet:demo:alice:voucher", Holder: alice, AssetCode: "VOUCHER", Amount: 30},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "wallet", BizType: "grant", BizNo: "wallet:demo:dave:mileage", Holder: dave, AssetCode: "MILEAGE", Amount: 1200},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "wallet", BizType: "grant", BizNo: "wallet:demo:closed:coin", Holder: closed, AssetCode: "COIN", Amount: 10},
	}
	for i := 1; i <= 12; i++ {
		grants = append(grants, domain.CommandRequest{
			Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant",
			BizNo: fmt.Sprintf("campaign:demo:vip:point:%02d", i), Holder: vip, AssetCode: "POINT", Amount: 10,
		})
	}
	if err := execDemo(ctx, books, grants); err != nil {
		return err
	}
	if books.limiter != nil {
		over := []domain.CommandRequest{
			{Command: domain.CmdDebit, TenantID: tenantID, SourceSystem: "wallet", BizType: "consume", BizNo: "wallet:demo:alice:debit:over", Holder: alice, AssetCode: "POINT", Amount: 200},
			{Command: domain.CmdDebit, TenantID: tenantID, SourceSystem: "wallet", BizType: "consume", BizNo: "wallet:demo:bob:debit:over", Holder: bob, AssetCode: "POINT", Amount: 90},
		}
		for _, req := range over {
			_, err := books.Execute(ctx, req)
			if err == nil {
				return fmt.Errorf("demo over-limit debit %s should be rejected", req.BizNo)
			}
			if !domain.Is(err, domain.CodeRateLimited) {
				return fmt.Errorf("%s: %w", req.BizNo, err)
			}
		}
	}
	steps := []domain.CommandRequest{
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:alice:fz:O1001", Holder: alice, AssetCode: "POINT", Amount: 2000, ExpireAt: &exp},
		{Command: domain.CmdCapture, TenantID: tenantID, SourceSystem: "order", BizType: "order_pay", BizNo: "order:demo:alice:cap:O1001", RelatedBizNo: "order:demo:alice:fz:O1001", Amount: 1500},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:alice:fz:O1002", Holder: alice, AssetCode: "POINT", Amount: 300, ExpireAt: &exp},
		{Command: domain.CmdRelease, TenantID: tenantID, SourceSystem: "order", BizType: "order_cancel", BizNo: "order:demo:alice:rel:O1002", RelatedBizNo: "order:demo:alice:fz:O1002"},
		{Command: domain.CmdTransfer, TenantID: tenantID, SourceSystem: "wallet", BizType: "gift", BizNo: "wallet:demo:alice:xfer:point", Holder: alice, ToHolder: &bob, AssetCode: "POINT", Amount: 800},
		{Command: domain.CmdReverse, TenantID: tenantID, SourceSystem: "wallet", BizType: "correct", BizNo: "wallet:demo:carol:reverse", RelatedBizNo: "campaign:demo:carol:point"},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:alice:fz:P2001", Holder: alice, AssetCode: "BALANCE_CNY", Amount: 80000, ExpireAt: &exp},
		{Command: domain.CmdCapture, TenantID: tenantID, SourceSystem: "order", BizType: "order_pay", BizNo: "order:demo:alice:cap:P2001", RelatedBizNo: "order:demo:alice:fz:P2001"},
		{Command: domain.CmdExchange, TenantID: tenantID, SourceSystem: "wallet", BizType: "fx", BizNo: "wallet:demo:alice:fx", Holder: alice, AssetCode: "BALANCE_CNY", Amount: 100000, ToAssetCode: "BALANCE_USD", ToAmount: 14000, FeeAsset: "BALANCE_CNY", FeeAmount: 100, Fx: &domain.FxQuote{BaseAsset: "BALANCE_CNY", QuoteAsset: "BALANCE_USD", Rate: "0.14000000", RateSource: "manual"}},
		{Command: domain.CmdTransfer, TenantID: tenantID, SourceSystem: "wallet", BizType: "settle", BizNo: "wallet:demo:alice:xfer:cny", Holder: alice, ToHolder: &shop, AssetCode: "BALANCE_CNY", Amount: 50000},
		{Command: domain.CmdDebit, TenantID: tenantID, SourceSystem: "wallet", BizType: "consume", BizNo: "wallet:demo:alice:debit:point", Holder: alice, AssetCode: "POINT", Amount: 50},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:alice:fz:COIN1", Holder: alice, AssetCode: "COIN", Amount: 1500, ExpireAt: &soon},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:alice:fz:COIN2", Holder: alice, AssetCode: "COIN", Amount: 400, ExpireAt: &exp},
		{Command: domain.CmdRelease, TenantID: tenantID, SourceSystem: "order", BizType: "order_cancel", BizNo: "order:demo:alice:rel:COIN2", RelatedBizNo: "order:demo:alice:fz:COIN2"},
		{Command: domain.CmdDebit, TenantID: tenantID, SourceSystem: "wallet", BizType: "consume", BizNo: "wallet:demo:alice:debit:coin", Holder: alice, AssetCode: "COIN", Amount: 50},
		{Command: domain.CmdTransfer, TenantID: tenantID, SourceSystem: "wallet", BizType: "gift", BizNo: "wallet:demo:alice:xfer:coin", Holder: alice, ToHolder: &bob, AssetCode: "COIN", Amount: 200},
		{Command: domain.CmdReverse, TenantID: tenantID, SourceSystem: "wallet", BizType: "correct", BizNo: "wallet:demo:dave:reverse", RelatedBizNo: "campaign:demo:dave:bonus"},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:dave:fz:D1", Holder: dave, AssetCode: "POINT", Amount: 1000, ExpireAt: &exp},
		{Command: domain.CmdCapture, TenantID: tenantID, SourceSystem: "order", BizType: "order_pay", BizNo: "order:demo:dave:cap:D1", RelatedBizNo: "order:demo:dave:fz:D1"},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:dave:fz:D2", Holder: dave, AssetCode: "POINT", Amount: 200, ExpireAt: &exp},
		{Command: domain.CmdRelease, TenantID: tenantID, SourceSystem: "order", BizType: "order_cancel", BizNo: "order:demo:dave:rel:D2", RelatedBizNo: "order:demo:dave:fz:D2"},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:dave:fz:D3", Holder: dave, AssetCode: "POINT", Amount: 150, ExpireAt: &soon},
		{Command: domain.CmdDebit, TenantID: tenantID, SourceSystem: "wallet", BizType: "consume", BizNo: "wallet:demo:dave:debit:point", Holder: dave, AssetCode: "POINT", Amount: 40},
		{Command: domain.CmdTransfer, TenantID: tenantID, SourceSystem: "wallet", BizType: "gift", BizNo: "wallet:demo:dave:xfer:frank", Holder: dave, ToHolder: &frank, AssetCode: "POINT", Amount: 300},
		{Command: domain.CmdExchange, TenantID: tenantID, SourceSystem: "wallet", BizType: "fx", BizNo: "wallet:demo:eve:fx", Holder: eve, AssetCode: "BALANCE_CNY", Amount: 100000, ToAssetCode: "BALANCE_HKD", ToAmount: 108000, FeeAsset: "BALANCE_CNY", FeeAmount: 100, Fx: &domain.FxQuote{BaseAsset: "BALANCE_CNY", QuoteAsset: "BALANCE_HKD", Rate: "1.08000000", RateSource: "feed"}},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:eve:fz:H1", Holder: eve, AssetCode: "BALANCE_HKD", Amount: 20000, ExpireAt: &soon},
		{Command: domain.CmdTransfer, TenantID: tenantID, SourceSystem: "wallet", BizType: "settle", BizNo: "wallet:demo:eve:xfer:cafe", Holder: eve, ToHolder: &cafe, AssetCode: "BALANCE_CNY", Amount: 30000},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:vip:fz:V1", Holder: vip, AssetCode: "POINT", Amount: 30, ExpireAt: &exp},
		{Command: domain.CmdFreeze, TenantID: "t_mall", SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:mall:fz:M1", Holder: buyer, AssetCode: "POINT", Amount: 400, ExpireAt: &exp},
		{Command: domain.CmdCapture, TenantID: "t_mall", SourceSystem: "order", BizType: "order_pay", BizNo: "order:demo:mall:cap:M1", RelatedBizNo: "order:demo:mall:fz:M1"},
		{Command: domain.CmdTransfer, TenantID: "t_mall", SourceSystem: "wallet", BizType: "settle", BizNo: "wallet:demo:mall:xfer:shop", Holder: buyer, ToHolder: &mallShop, AssetCode: "POINT", Amount: 200},
		{Command: domain.CmdFreeze, TenantID: "t_game", SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:gamer:fz:G1", Holder: gamer, AssetCode: "POINT", Amount: 800, ExpireAt: &soon},
		{Command: domain.CmdDebit, TenantID: "t_game", SourceSystem: "wallet", BizType: "consume", BizNo: "wallet:demo:gamer:debit", Holder: gamer, AssetCode: "POINT", Amount: 50},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:alice:fz:EXPIRED", Holder: alice, AssetCode: "POINT", Amount: 100, ExpireAt: &past},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:dave:fz:EXPIRED", Holder: dave, AssetCode: "POINT", Amount: 50, ExpireAt: &past},
		{Command: domain.CmdFreeze, TenantID: tenantID, SourceSystem: "order", BizType: "order_hold", BizNo: "order:demo:eve:fz:EXPIRED", Holder: eve, AssetCode: "BALANCE_HKD", Amount: 8000, ExpireAt: &past},
	}
	if err := execDemo(ctx, books, steps); err != nil {
		return err
	}

	if accounts != nil {
		dormantAcc, err := accounts.Get(ctx, tenantID, dormant, "POINT")
		if err != nil {
			return err
		}
		if _, err := accounts.SetStatus(ctx, tenantID, dormantAcc.AccountID, domain.AccountDisabled); err != nil {
			return err
		}
		closedAcc, err := accounts.Get(ctx, tenantID, closed, "COIN")
		if err != nil {
			return err
		}
		if _, err := accounts.SetStatus(ctx, tenantID, closedAcc.AccountID, domain.AccountDisabled); err != nil {
			return err
		}
	}

	if recon != nil {
		if err := seedDemoReconcile(ctx, recon, tenantID); err != nil {
			return err
		}
	}
	if err := seedDemoSagas(ctx, sagas, tenantID); err != nil {
		return err
	}
	return nil
}

func execDemo(ctx context.Context, books *Bookkeeping, reqs []domain.CommandRequest) error {
	for _, req := range reqs {
		if _, err := books.Execute(ctx, req); err != nil {
			return fmt.Errorf("%s %s: %w", req.Command, req.BizNo, err)
		}
	}
	return nil
}

func seedDemoReconcile(ctx context.Context, recon *ReconcileService, tenantID string) error {
	date := time.Now().UTC().Format("2006-01-02")
	campaignLines := []domain.BizLine{
		{BizNo: "campaign:demo:alice:point", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 10000},
		{BizNo: "campaign:demo:bob:point", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 3000},
		{BizNo: "campaign:demo:carol:point", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 200},
		{BizNo: "campaign:demo:dormant:point", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 120},
		{BizNo: "campaign:demo:dave:point", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 5000},
		{BizNo: "campaign:demo:dave:bonus", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 80},
		{BizNo: "campaign:demo:dave:checkin", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 200},
		{BizNo: "campaign:demo:dave:task", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 150},
		{BizNo: "campaign:demo:ghost", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 50},
	}
	for i := 1; i <= 12; i++ {
		campaignLines = append(campaignLines, domain.BizLine{
			BizNo: fmt.Sprintf("campaign:demo:vip:point:%02d", i), Command: domain.CmdCredit, AssetCode: "POINT", Amount: 10,
		})
	}
	if err := triggerIfMissing(ctx, recon, tenantID, date, "campaign", "POINT", campaignLines, nil); err != nil {
		return err
	}
	if err := triggerIfMissing(ctx, recon, tenantID, date, "pay", "BALANCE_CNY", nil, []domain.BizLine{
		{BizNo: "pay:demo:alice:cny", Command: domain.CmdCredit, AssetCode: "BALANCE_CNY", Amount: 1280000},
		{BizNo: "pay:demo:bob:cny", Command: domain.CmdCredit, AssetCode: "BALANCE_CNY", Amount: 200000},
		{BizNo: "pay:demo:eve:cny", Command: domain.CmdCredit, AssetCode: "BALANCE_CNY", Amount: 200000},
	}); err != nil {
		return err
	}
	if err := triggerIfMissing(ctx, recon, tenantID, date, "wallet", "POINT", []domain.BizLine{
		{BizNo: "wallet:demo:alice:xfer:point", Command: domain.CmdTransfer, AssetCode: "POINT", Amount: 800},
		{BizNo: "wallet:demo:alice:debit:point", Command: domain.CmdDebit, AssetCode: "POINT", Amount: 80},
		{BizNo: "wallet:demo:dave:debit:point", Command: domain.CmdDebit, AssetCode: "POINT", Amount: 40},
		{BizNo: "wallet:demo:dave:xfer:frank", Command: domain.CmdTransfer, AssetCode: "POINT", Amount: 300},
		{BizNo: "wallet:demo:phantom", Command: domain.CmdDebit, AssetCode: "POINT", Amount: 15},
	}, nil); err != nil {
		return err
	}
	if err := triggerIfMissing(ctx, recon, "t_mall", date, "campaign", "POINT", []domain.BizLine{
		{BizNo: "campaign:demo:mall:point", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 2000},
		{BizNo: "campaign:demo:mall:coupon", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 500},
		{BizNo: "campaign:demo:mall:ghost", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 10},
	}, nil); err != nil {
		return err
	}
	return decorateDemoDiffs(ctx, recon, tenantID, "t_mall")
}

func triggerIfMissing(ctx context.Context, recon *ReconcileService, tenantID, date, sys, asset string, biz, channel []domain.BizLine) error {
	if _, err := recon.ReportByDate(ctx, tenantID, date, sys, asset); err == nil {
		return nil
	}
	if _, err := recon.Trigger(ctx, tenantID, date, sys, asset, biz, channel); err != nil {
		return fmt.Errorf("demo reconcile %s %s %s: %w", tenantID, sys, asset, err)
	}
	return nil
}

func decorateDemoDiffs(ctx context.Context, recon *ReconcileService, tenantIDs ...string) error {
	for _, tid := range tenantIDs {
		open, err := recon.ListOpenDiffs(ctx, tid, 100)
		if err != nil {
			return err
		}
		for _, d := range open {
			switch {
			case d.BizNo == "campaign:demo:ghost" && d.Assignee == "":
				if _, err := recon.AssignDiff(ctx, tid, d.DiffID, "ops_zhang", "演示：业务有账本无，待补记账", "demo"); err != nil {
					return err
				}
			case d.BizNo == "campaign:demo:mall:ghost" && d.Assignee == "":
				if _, err := recon.AssignDiff(ctx, tid, d.DiffID, "ops_wang", "演示：商城少账", "demo"); err != nil {
					return err
				}
			case d.BizNo == "wallet:demo:phantom" && d.Assignee == "":
				if _, err := recon.AssignDiff(ctx, tid, d.DiffID, "ops_li", "演示：渠道单边", "demo"); err != nil {
					return err
				}
			case d.BizNo == "wallet:demo:alice:debit:point" && d.Kind == domain.DiffAmountMismatch:
				if err := recon.ResolveDiff(ctx, tid, d.DiffID, "演示关单：已与钱包核对，账本 50 为准", "ops_li"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func seedDemoSagas(ctx context.Context, sagas domain.SagaRepository, tenantID string) error {
	if sagas == nil || tenantID == "" {
		return nil
	}
	now := time.Now().UTC()
	demos := []*domain.TransferSaga{
		{
			SagaID: "sg_demo_pending", TenantID: tenantID, SourceSystem: "wallet",
			BizNo: "wallet:demo:saga:pending", FromType: domain.HolderUser, FromID: "u_saga_from",
			ToType: domain.HolderUser, ToID: "u_saga_to", AssetCode: "POINT", Amount: 100,
			Status: domain.SagaPending, ResultJSON: domain.DemoSagaJSON,
			LastError: "演示：跨分片转账尚未扣出账", CreatedAt: now.Add(-20 * time.Minute),
		},
		{
			SagaID: "sg_demo_out_done", TenantID: tenantID, SourceSystem: "wallet",
			BizNo: "wallet:demo:saga:out_done", FromType: domain.HolderUser, FromID: "u_saga_from",
			ToType: domain.HolderUser, ToID: "u_saga_to", AssetCode: "POINT", Amount: 80,
			Status: domain.SagaOutDone, OutBizNo: "wallet:demo:saga:out_done:out",
			ResultJSON: domain.DemoSagaJSON, LastError: "演示：出账完成，入账未达",
			CreatedAt: now.Add(-12 * time.Minute),
		},
		{
			SagaID: "sg_demo_compensating", TenantID: tenantID, SourceSystem: "wallet",
			BizNo: "wallet:demo:saga:compensating", FromType: domain.HolderUser, FromID: "u_saga_from",
			ToType: domain.HolderUser, ToID: "u_saga_to", AssetCode: "POINT", Amount: 50,
			Status: domain.SagaCompensating, OutBizNo: "wallet:demo:saga:compensating:out",
			RollbackNo: "wallet:demo:saga:compensating:rollback", ResultJSON: domain.DemoSagaJSON,
			LastError: "演示：入账失败，补偿中", CreatedAt: now.Add(-5 * time.Minute),
		},
	}
	for _, sg := range demos {
		existing, err := sagas.GetByBizNo(ctx, sg.TenantID, sg.SourceSystem, sg.BizNo)
		if err == nil && existing != nil {
			continue
		}
		if err != nil && !domain.Is(err, domain.CodeNotFound) {
			return err
		}
		if err := sagas.Create(ctx, sg); err != nil {
			return fmt.Errorf("demo saga %s: %w", sg.BizNo, err)
		}
	}
	return nil
}
