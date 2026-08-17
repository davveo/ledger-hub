package application

import (
	"context"
	"testing"

	"github.com/davveo/ledger-hub/internal/domain"
)

type memLimit struct{}

func (memLimit) AddUsage(_ context.Context, _, _, _, _ string, _ domain.Command, _ string, amount int64) (int64, int, error) {
	return amount, 1, nil
}

func TestSeedDemoBalances(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	b.UsePhase3(nil, nil, nil, NewLimiter([]domain.LimitRule{{
		SourceSystem: "wallet", AssetCode: "POINT", Command: domain.CmdDebit, MaxAmount: 80,
	}}, memLimit{}), nil)
	assets := NewAssetService(st)
	accs := NewAccountService(st, memAccount{st})
	if err := SeedDemo(ctx, b, assets, accs, nil, nil, nil, "t_default"); err != nil {
		t.Fatal(err)
	}
	if err := SeedDemo(ctx, b, assets, accs, nil, nil, nil, "t_default"); err != nil {
		t.Fatal(err)
	}

	alice := domain.Holder{Type: domain.HolderUser, ID: "u_alice"}
	bob := domain.Holder{Type: domain.HolderUser, ID: "u_bob"}
	carol := domain.Holder{Type: domain.HolderUser, ID: "u_carol"}
	got, err := memAccount{st}.Get(ctx, "t_default", alice, "POINT")
	if err != nil {
		t.Fatal(err)
	}
	if got.Available != 7150 || got.Frozen != 500 {
		t.Fatalf("alice POINT avail=%d frozen=%d", got.Available, got.Frozen)
	}
	cny, err := memAccount{st}.Get(ctx, "t_default", alice, "BALANCE_CNY")
	if err != nil {
		t.Fatal(err)
	}
	if cny.Available != 1049900 || cny.Frozen != 0 {
		t.Fatalf("alice CNY avail=%d frozen=%d", cny.Available, cny.Frozen)
	}
	usd, err := memAccount{st}.Get(ctx, "t_default", alice, "BALANCE_USD")
	if err != nil {
		t.Fatal(err)
	}
	if usd.Available != 14000 {
		t.Fatalf("alice USD avail=%d", usd.Available)
	}
	bp, err := memAccount{st}.Get(ctx, "t_default", bob, "POINT")
	if err != nil {
		t.Fatal(err)
	}
	if bp.Available != 3800 {
		t.Fatalf("bob POINT avail=%d", bp.Available)
	}
	cp, err := memAccount{st}.Get(ctx, "t_default", carol, "POINT")
	if err != nil {
		t.Fatal(err)
	}
	if cp.Available != 0 {
		t.Fatalf("carol POINT after reverse avail=%d", cp.Available)
	}
	growth, err := assets.Get(ctx, "t_default", "GROWTH")
	if err != nil || growth.Status != domain.AssetDisabled {
		t.Fatalf("GROWTH should be disabled: %+v %v", growth, err)
	}
	dormant, err := memAccount{st}.Get(ctx, "t_default", domain.Holder{Type: domain.HolderUser, ID: "u_dormant"}, "POINT")
	if err != nil || dormant.Status != domain.AccountDisabled || dormant.Available != 120 {
		t.Fatalf("dormant %+v err=%v", dormant, err)
	}
	alerts := b.limiter.Alerts()
	if len(alerts) == 0 || alerts[0].Reason == "" {
		t.Fatalf("want limit alert, got %+v", alerts)
	}
}
