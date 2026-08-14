package application

import (
	"context"

	"github.com/davveo/ledger-hub/internal/domain"
)

func SeedAssets(ctx context.Context, svc *AssetService, tenantID string) error {
	if svc == nil || tenantID == "" {
		return nil
	}
	seeds := []*domain.Asset{
		{TenantID: tenantID, AssetCode: "POINT", Name: "积分", AssetClass: "point", Precision: 0, FreezeSupported: true, Status: domain.AssetActive, Ext: `{"expire":{"policy":"year_end"}}`},
		{TenantID: tenantID, AssetCode: "BALANCE_CNY", Name: "人民币余额", AssetClass: "fiat", CurrencyCode: "CNY", Precision: 2, FreezeSupported: true, Status: domain.AssetActive},
		{TenantID: tenantID, AssetCode: "BALANCE_USD", Name: "美元余额", AssetClass: "fiat", CurrencyCode: "USD", Precision: 2, FreezeSupported: true, Status: domain.AssetActive},
		{TenantID: tenantID, AssetCode: "BALANCE_HKD", Name: "港币余额", AssetClass: "fiat", CurrencyCode: "HKD", Precision: 2, FreezeSupported: true, Status: domain.AssetActive},
		{TenantID: tenantID, AssetCode: "COIN", Name: "金币", AssetClass: "token", Precision: 0, FreezeSupported: true, Status: domain.AssetActive},
		{TenantID: tenantID, AssetCode: "GROWTH", Name: "成长值", AssetClass: "point", Precision: 0, Status: domain.AssetActive},
	}
	for _, a := range seeds {
		if _, err := svc.Get(ctx, tenantID, a.AssetCode); err == nil {
			continue
		}
		if err := svc.Save(ctx, a); err != nil {
			return err
		}
	}
	return nil
}
