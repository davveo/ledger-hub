package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

type ExpireEngine struct {
	assets  domain.AssetRepository
	accs    domain.AccountRepository
	entries domain.EntryRepository
	books   *Bookkeeping
}

func NewExpireEngine(assets domain.AssetRepository, accs domain.AccountRepository, entries domain.EntryRepository, books *Bookkeeping) *ExpireEngine {
	return &ExpireEngine{assets: assets, accs: accs, entries: entries, books: books}
}

type expirePolicy struct {
	Policy string `json:"policy"`
	Days   int    `json:"days"`
}

func (e *ExpireEngine) Run(ctx context.Context, tenantID string, now time.Time) (int, error) {
	if e == nil || e.books == nil {
		return 0, nil
	}
	assets, err := e.assets.List(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, a := range assets {
		pol := parseExpire(a.Ext)
		if pol.Policy == "" {
			continue
		}
		accs, err := e.accs.ListByTenant(ctx, tenantID, a.AssetCode)
		if err != nil {
			return n, err
		}
		for _, acc := range accs {
			if acc.HolderType == domain.HolderSystemSubject || acc.Available <= 0 {
				continue
			}
			amt, err := e.expireAmount(ctx, acc, pol, now)
			if err != nil {
				return n, err
			}
			if amt <= 0 {
				continue
			}
			bizNo := "worker:expire:" + a.AssetCode + ":" + acc.HolderID + ":" + now.UTC().Format("2006-01-02")
			sink := domain.Holder{Type: domain.HolderSystemSubject, ID: domain.SystemPointSink}
			_, err = e.books.Execute(ctx, domain.CommandRequest{
				Command:      domain.CmdTransfer,
				TenantID:     tenantID,
				SourceSystem: "worker",
				BizType:      "expire",
				BizNo:        bizNo,
				Holder:       domain.Holder{Type: acc.HolderType, ID: acc.HolderID},
				ToHolder:     &sink,
				AssetCode:    a.AssetCode,
				Amount:       amt,
			})
			if err != nil {
				if domain.Is(err, domain.CodeIdempotencyConflict) {
					continue
				}
				return n, err
			}
			n++
		}
	}
	return n, nil
}

func (e *ExpireEngine) expireAmount(ctx context.Context, acc *domain.Account, pol expirePolicy, now time.Time) (int64, error) {
	switch pol.Policy {
	case "year_end":
		if now.Month() == time.January && now.Day() <= 2 {
			return acc.Available, nil
		}
		return 0, nil
	case "rolling_days":
		days := pol.Days
		if days <= 0 {
			days = 365
		}
		cutoff := now.UTC().Add(-time.Duration(days) * 24 * time.Hour)
		hist, err := e.entries.ListByAccount(ctx, acc.AccountID)
		if err != nil {
			return 0, err
		}
		var lots int64
		for _, en := range hist {
			if en.Command != domain.CmdCredit || en.Direction != domain.DirIN {
				continue
			}
			if !en.CreatedAt.Before(cutoff) {
				lots += en.Amount
			}
		}
		if acc.Available > lots {
			return acc.Available - lots, nil
		}
		return 0, nil
	default:
		return 0, nil
	}
}

func parseExpire(ext string) expirePolicy {
	if ext == "" {
		return expirePolicy{}
	}
	var wrap struct {
		Expire expirePolicy `json:"expire"`
	}
	if json.Unmarshal([]byte(ext), &wrap) != nil {
		return expirePolicy{}
	}
	return wrap.Expire
}
