package application

import (
	"testing"

	"github.com/davveo/ledger-hub/internal/domain"
)

func TestMatchBizLines(t *testing.T) {
	entries := []*domain.LedgerEntry{
		{BizNo: "order:freeze:O1", Command: domain.CmdFreeze, AssetCode: "POINT", Amount: 50},
		{BizNo: "order:capture:O1", Command: domain.CmdCapture, AssetCode: "POINT", Amount: 50},
	}
	biz := []domain.BizLine{
		{BizNo: "order:freeze:O1", Command: domain.CmdFreeze, AssetCode: "POINT", Amount: 50},
		{BizNo: "order:capture:O1", Command: domain.CmdCapture, AssetCode: "POINT", Amount: 50},
	}
	diffs := MatchBizLines("job", biz, entries)
	if len(diffs) != 0 {
		t.Fatalf("want no diffs, got %+v", diffs)
	}

	missing := MatchBizLines("job", append(biz, domain.BizLine{
		BizNo: "order:capture:O2", Command: domain.CmdCapture, AssetCode: "POINT", Amount: 20,
	}), entries)
	if len(missing) != 1 || missing[0].Kind != domain.DiffMissing {
		t.Fatalf("want missing, got %+v", missing)
	}

	amt := MatchBizLines("job", []domain.BizLine{
		{BizNo: "order:freeze:O1", Command: domain.CmdFreeze, AssetCode: "POINT", Amount: 80},
	}, entries)
	foundAmt := false
	for _, d := range amt {
		if d.Kind == domain.DiffAmountMismatch {
			foundAmt = true
		}
	}
	if !foundAmt {
		t.Fatalf("want amount mismatch, got %+v", amt)
	}
}

func TestReconstruct(t *testing.T) {
	avail, frozen := reconstruct([]*domain.LedgerEntry{
		{Command: domain.CmdCredit, Amount: 100, Direction: domain.DirIN},
		{Command: domain.CmdFreeze, Amount: 40, Direction: domain.DirOUT},
		{Command: domain.CmdCapture, Amount: 40, Direction: domain.DirOUT},
	})
	if avail != 60 || frozen != 0 {
		t.Fatalf("got available=%d frozen=%d", avail, frozen)
	}
}
