package application

import (
	"context"
	"fmt"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

// SeedDemo 写入一套可重复执行的演示账，全部走记账命令，余额 / 冻结 / 流水自洽。
// 覆盖运营台：租户、资产、多币种账户、流水、冻结、汇率、冲正、对账差异、限额告警。
func SeedDemo(ctx context.Context, books *Bookkeeping, assets *AssetService, accounts *AccountService, tenants *TenantService, fx *FxService, recon *ReconcileService, tenantID string) error {
	if books == nil || assets == nil || tenantID == "" {
		return nil
	}
	if err := SeedAssets(ctx, assets, tenantID); err != nil {
		return err
	}
	if err := SeedAssets(ctx, assets, "t_mall"); err != nil {
		return err
	}
	if err := SeedAssets(ctx, assets, "t_legacy"); err != nil {
		return err
	}
	if tenants != nil {
		for _, t := range []*domain.Tenant{
			{TenantID: tenantID, Name: "默认租户", Status: "active"},
			{TenantID: "t_mall", Name: "商城租户", Status: "active"},
			{TenantID: "t_legacy", Name: "历史租户", Status: "disabled"},
		} {
			if err := tenants.Save(ctx, t); err != nil {
				return err
			}
		}
	}
	if fx != nil {
		rates := []*domain.FxRate{
			{RateID: "fxr_demo_cny_usd", TenantID: tenantID, BaseAsset: "BALANCE_CNY", QuoteAsset: "BALANCE_USD", Rate: "0.14000000", RateSource: "manual", CreatedBy: "demo"},
			{RateID: "fxr_demo_usd_cny", TenantID: tenantID, BaseAsset: "BALANCE_USD", QuoteAsset: "BALANCE_CNY", Rate: "7.14285714", RateSource: "manual", CreatedBy: "demo"},
			{RateID: "fxr_demo_cny_hkd", TenantID: tenantID, BaseAsset: "BALANCE_CNY", QuoteAsset: "BALANCE_HKD", Rate: "1.08000000", RateSource: "feed", CreatedBy: "demo"},
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

	alice := domain.Holder{Type: domain.HolderUser, ID: "u_alice"}
	bob := domain.Holder{Type: domain.HolderUser, ID: "u_bob"}
	carol := domain.Holder{Type: domain.HolderUser, ID: "u_carol"}
	dormant := domain.Holder{Type: domain.HolderUser, ID: "u_dormant"}
	shop := domain.Holder{Type: domain.HolderMerchant, ID: "m_shop"}
	buyer := domain.Holder{Type: domain.HolderUser, ID: "u_buyer"}
	exp := time.Now().UTC().Add(72 * time.Hour)

	grants := []domain.CommandRequest{
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:alice:point", Holder: alice, AssetCode: "POINT", Amount: 10000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:bob:point", Holder: bob, AssetCode: "POINT", Amount: 3000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:carol:point", Holder: carol, AssetCode: "POINT", Amount: 200},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:dormant:point", Holder: dormant, AssetCode: "POINT", Amount: 120},
		{Command: domain.CmdCredit, TenantID: "t_mall", SourceSystem: "campaign", BizType: "grant", BizNo: "campaign:demo:mall:point", Holder: buyer, AssetCode: "POINT", Amount: 2000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "pay", BizType: "topup", BizNo: "pay:demo:alice:cny", Holder: alice, AssetCode: "BALANCE_CNY", Amount: 1280000},
		{Command: domain.CmdCredit, TenantID: tenantID, SourceSystem: "pay", BizType: "topup", BizNo: "pay:demo:bob:cny", Holder: bob, AssetCode: "BALANCE_CNY", Amount: 200000},
	}
	for _, req := range grants {
		if _, err := books.Execute(ctx, req); err != nil {
			return fmt.Errorf("%s %s: %w", req.Command, req.BizNo, err)
		}
	}
	if books.limiter != nil {
		_, err := books.Execute(ctx, domain.CommandRequest{
			Command: domain.CmdDebit, TenantID: tenantID, SourceSystem: "wallet", BizType: "consume",
			BizNo: "wallet:demo:alice:debit:over", Holder: alice, AssetCode: "POINT", Amount: 200,
		})
		if err == nil {
			return fmt.Errorf("demo over-limit debit should be rejected")
		}
		if !domain.Is(err, domain.CodeRateLimited) {
			return fmt.Errorf("wallet:demo:alice:debit:over: %w", err)
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
	}
	for _, req := range steps {
		if _, err := books.Execute(ctx, req); err != nil {
			return fmt.Errorf("%s %s: %w", req.Command, req.BizNo, err)
		}
	}

	dormantAcc, err := accounts.Get(ctx, tenantID, dormant, "POINT")
	if err != nil {
		return err
	}
	if _, err := accounts.SetStatus(ctx, dormantAcc.AccountID, domain.AccountDisabled); err != nil {
		return err
	}

	if recon != nil {
		date := time.Now().UTC().Format("2006-01-02")
		if _, err := recon.ReportByDate(ctx, tenantID, date, "campaign", "POINT"); err != nil {
			_, err = recon.Trigger(ctx, tenantID, date, "campaign", "POINT", []domain.BizLine{
				{BizNo: "campaign:demo:alice:point", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 10000},
				{BizNo: "campaign:demo:bob:point", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 3000},
				{BizNo: "campaign:demo:carol:point", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 200},
				{BizNo: "campaign:demo:dormant:point", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 120},
				{BizNo: "campaign:demo:ghost", Command: domain.CmdCredit, AssetCode: "POINT", Amount: 50},
			}, nil)
			if err != nil {
				return fmt.Errorf("demo reconcile: %w", err)
			}
		}
		if _, err := recon.ReportByDate(ctx, tenantID, date, "pay", "BALANCE_CNY"); err != nil {
			_, err = recon.Trigger(ctx, tenantID, date, "pay", "BALANCE_CNY", nil, []domain.BizLine{
				{BizNo: "pay:demo:alice:cny", Command: domain.CmdCredit, AssetCode: "BALANCE_CNY", Amount: 1280000},
				{BizNo: "pay:demo:bob:cny", Command: domain.CmdCredit, AssetCode: "BALANCE_CNY", Amount: 200000},
			})
			if err != nil {
				return fmt.Errorf("demo pay reconcile: %w", err)
			}
		}
	}
	return nil
}
