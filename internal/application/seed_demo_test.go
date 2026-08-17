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
	if got.Available != 7050 || got.Frozen != 600 {
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
	if len(alerts) < 2 {
		t.Fatalf("want at least 2 limit alerts, got %+v", alerts)
	}

	coin, err := memAccount{st}.Get(ctx, "t_default", alice, "COIN")
	if err != nil {
		t.Fatal(err)
	}
	if coin.Available != 6250 || coin.Frozen != 1500 {
		t.Fatalf("alice COIN avail=%d frozen=%d", coin.Available, coin.Frozen)
	}
	voucher, err := memAccount{st}.Get(ctx, "t_default", alice, "VOUCHER")
	if err != nil || voucher.Available != 30 {
		t.Fatalf("alice VOUCHER %+v err=%v", voucher, err)
	}

	dave := domain.Holder{Type: domain.HolderUser, ID: "u_dave"}
	dp, err := memAccount{st}.Get(ctx, "t_default", dave, "POINT")
	if err != nil {
		t.Fatal(err)
	}
	if dp.Available != 3810 || dp.Frozen != 200 {
		t.Fatalf("dave POINT avail=%d frozen=%d", dp.Available, dp.Frozen)
	}
	mile, err := memAccount{st}.Get(ctx, "t_default", dave, "MILEAGE")
	if err != nil || mile.Available != 1200 {
		t.Fatalf("dave MILEAGE %+v err=%v", mile, err)
	}

	eve := domain.Holder{Type: domain.HolderUser, ID: "u_eve"}
	eveCNY, err := memAccount{st}.Get(ctx, "t_default", eve, "BALANCE_CNY")
	if err != nil {
		t.Fatal(err)
	}
	if eveCNY.Available != 69900 {
		t.Fatalf("eve CNY avail=%d", eveCNY.Available)
	}
	eveHKD, err := memAccount{st}.Get(ctx, "t_default", eve, "BALANCE_HKD")
	if err != nil {
		t.Fatal(err)
	}
	if eveHKD.Available != 80000 || eveHKD.Frozen != 28000 {
		t.Fatalf("eve HKD avail=%d frozen=%d", eveHKD.Available, eveHKD.Frozen)
	}

	vip, err := memAccount{st}.Get(ctx, "t_default", domain.Holder{Type: domain.HolderUser, ID: "u_vip"}, "POINT")
	if err != nil || vip.Available != 90 || vip.Frozen != 30 {
		t.Fatalf("vip POINT %+v err=%v", vip, err)
	}
	frank, err := memAccount{st}.Get(ctx, "t_default", domain.Holder{Type: domain.HolderUser, ID: "u_frank"}, "POINT")
	if err != nil || frank.Available != 300 {
		t.Fatalf("frank POINT %+v err=%v", frank, err)
	}
	bobUSD, err := memAccount{st}.Get(ctx, "t_default", bob, "BALANCE_USD")
	if err != nil || bobUSD.Available != 5000 {
		t.Fatalf("bob USD %+v err=%v", bobUSD, err)
	}
	bobCoin, err := memAccount{st}.Get(ctx, "t_default", bob, "COIN")
	if err != nil || bobCoin.Available != 200 {
		t.Fatalf("bob COIN %+v err=%v", bobCoin, err)
	}
	cafe, err := memAccount{st}.Get(ctx, "t_default", domain.Holder{Type: domain.HolderMerchant, ID: "m_cafe"}, "BALANCE_CNY")
	if err != nil || cafe.Available != 30000 {
		t.Fatalf("cafe CNY %+v err=%v", cafe, err)
	}
	closed, err := memAccount{st}.Get(ctx, "t_default", domain.Holder{Type: domain.HolderUser, ID: "u_closed"}, "COIN")
	if err != nil || closed.Status != domain.AccountDisabled || closed.Available != 10 {
		t.Fatalf("closed %+v err=%v", closed, err)
	}

	buyer := domain.Holder{Type: domain.HolderUser, ID: "u_buyer"}
	mall, err := memAccount{st}.Get(ctx, "t_mall", buyer, "POINT")
	if err != nil || mall.Available != 1900 {
		t.Fatalf("mall buyer POINT %+v err=%v", mall, err)
	}
	gamer, err := memAccount{st}.Get(ctx, "t_game", domain.Holder{Type: domain.HolderUser, ID: "u_gamer"}, "POINT")
	if err != nil || gamer.Available != 11150 || gamer.Frozen != 800 {
		t.Fatalf("gamer POINT %+v err=%v", gamer, err)
	}
}
