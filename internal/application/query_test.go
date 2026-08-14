package application

import (
	"context"
	"testing"

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
