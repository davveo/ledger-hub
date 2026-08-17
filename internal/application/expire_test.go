package application

import (
	"context"
	"testing"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

func TestExpireYearEnd(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	_ = st.Save(ctx, &domain.Asset{
		TenantID: "t_default", AssetCode: "POINT", Name: "积分", Status: domain.AssetActive,
		Ext: `{"expire":{"policy":"year_end"}}`,
	})
	holder := domain.Holder{Type: domain.HolderUser, ID: "u_exp"}
	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "campaign",
		BizNo: "campaign:exp", Holder: holder, AssetCode: "POINT", Amount: 80,
	}); err != nil {
		t.Fatal(err)
	}
	eng := NewExpireEngine(st, memAccount{st}, memEntry{st}, b)
	n, err := eng.Run(ctx, "t_default", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 expire, got %d", n)
	}
	acc, err := memAccount{st}.Get(ctx, "t_default", holder, "POINT")
	if err != nil {
		t.Fatal(err)
	}
	if acc.Available != 0 {
		t.Fatalf("available=%d", acc.Available)
	}
	sink, err := memAccount{st}.Get(ctx, "t_default", domain.Holder{Type: domain.HolderSystemSubject, ID: domain.SystemPointSink}, "POINT")
	if err != nil {
		t.Fatal(err)
	}
	if sink.Available != 80 {
		t.Fatalf("sink=%d", sink.Available)
	}
}

func TestExpirePreviewDoesNotPost(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	_ = st.Save(ctx, &domain.Asset{
		TenantID: "t_default", AssetCode: "POINT", Name: "积分", Status: domain.AssetActive,
		Ext: `{"expire":{"policy":"year_end"}}`,
	})
	holder := domain.Holder{Type: domain.HolderUser, ID: "u_prev"}
	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "campaign",
		BizNo: "campaign:prev", Holder: holder, AssetCode: "POINT", Amount: 80,
	}); err != nil {
		t.Fatal(err)
	}
	eng := NewExpireEngine(st, memAccount{st}, memEntry{st}, b)
	items, err := eng.Preview(ctx, "t_default", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(items) != 1 || items[0].Amount != 80 {
		t.Fatalf("preview %+v err=%v", items, err)
	}
	acc, err := memAccount{st}.Get(ctx, "t_default", holder, "POINT")
	if err != nil || acc.Available != 80 {
		t.Fatalf("preview must not debit available=%v err=%v", acc, err)
	}
}
