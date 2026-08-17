package application

import (
	"context"
	"testing"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

func TestSeedAssets(t *testing.T) {
	st := newMem()
	svc := NewAssetService(st)
	if err := SeedAssets(context.Background(), svc, "t_default"); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(context.Background(), "t_default")
	if err != nil || len(list) < 6 {
		t.Fatalf("want 6 seeds, got %d err=%v", len(list), err)
	}
	if err := SeedAssets(context.Background(), svc, "t_default"); err != nil {
		t.Fatal(err)
	}
	list2, _ := svc.List(context.Background(), "t_default")
	if len(list2) != len(list) {
		t.Fatalf("seed should be idempotent %d vs %d", len(list2), len(list))
	}
}

func TestListByHolderAndPage(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	holder := domain.Holder{Type: domain.HolderUser, ID: "u_q"}
	for i := 0; i < 3; i++ {
		if _, err := b.Execute(ctx, domain.CommandRequest{
			Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "campaign",
			BizNo: "campaign:q" + string(rune('a'+i)), Holder: holder, AssetCode: "POINT", Amount: 10,
		}); err != nil {
			t.Fatal(err)
		}
	}
	accs, err := memAccount{st}.ListByHolder(ctx, "t_default", holder, "")
	if err != nil || len(accs) != 1 {
		t.Fatalf("accounts=%d err=%v", len(accs), err)
	}
	q := NewQueryService(memEntry{st}, memFreeze{st})
	page, err := q.EntriesByHolder(ctx, "t_default", holder, "POINT", nil, nil, domain.Page{Limit: 2})
	if err != nil || len(page) != 2 {
		t.Fatalf("page=%d err=%v", len(page), err)
	}
}

func TestACLReplaceAndTenant(t *testing.T) {
	acl := NewACL([]domain.ACLRule{{SourceSystem: "campaign", Commands: []string{"Credit"}, Assets: []string{"POINT"}}})
	if !acl.Allow("campaign", domain.CmdCredit, "POINT") {
		t.Fatal("want allow")
	}
	acl.Replace([]domain.ACLRule{{TenantID: "t_a", SourceSystem: "campaign", Commands: []string{"Credit"}, Assets: []string{"POINT"}}})
	if acl.AllowTenant("t_b", "campaign", domain.CmdCredit, "POINT") {
		t.Fatal("other tenant should deny")
	}
	if !acl.AllowTenant("t_a", "campaign", domain.CmdCredit, "POINT") {
		t.Fatal("same tenant should allow")
	}
}

func TestExpiredFreezes(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	holder := domain.Holder{Type: domain.HolderUser, ID: "u_exp_fz"}
	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "campaign",
		BizNo: "campaign:expfz", Holder: holder, AssetCode: "POINT", Amount: 80,
	}); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	res, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdFreeze, TenantID: "t_default", SourceSystem: "order",
		BizNo: "order:expfz", Holder: holder, AssetCode: "POINT", Amount: 20, ExpireAt: &past,
	})
	if err != nil {
		t.Fatal(err)
	}
	q := NewQueryService(memEntry{st}, memFreeze{st})
	list, err := q.ExpiredFreezes(ctx, "t_default", time.Now().UTC(), 10)
	if err != nil || len(list) != 1 || list[0].FreezeID != res.FreezeID {
		t.Fatalf("expired=%+v err=%v want %s", list, err, res.FreezeID)
	}
}

func TestCrossTenantLookupNotFound(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	holder := domain.Holder{Type: domain.HolderUser, ID: "u_xtenant"}
	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "campaign",
		BizNo: "campaign:xt", Holder: holder, AssetCode: "POINT", Amount: 10,
	}); err != nil {
		t.Fatal(err)
	}
	accs, err := memAccount{st}.ListByHolder(ctx, "t_default", holder, "POINT")
	if err != nil || len(accs) != 1 {
		t.Fatalf("accounts=%d err=%v", len(accs), err)
	}
	svc := NewAccountService(st, memAccount{st})
	_, err = svc.GetByID(ctx, "t_other", accs[0].AccountID)
	if !domain.Is(err, domain.CodeNotFound) {
		t.Fatalf("cross-tenant account want 404, got %v", err)
	}
	fz, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdFreeze, TenantID: "t_default", SourceSystem: "order",
		BizNo: "order:xt", Holder: holder, AssetCode: "POINT", Amount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	q := NewQueryService(memEntry{st}, memFreeze{st})
	_, err = q.FreezeByID(ctx, "t_other", fz.FreezeID)
	if !domain.Is(err, domain.CodeNotFound) {
		t.Fatalf("cross-tenant freeze want 404, got %v", err)
	}
}
