package application

import (
	"context"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

func (s *Bookkeeping) reverse(ctx context.Context, req domain.CommandRequest) (*domain.CommandResult, error) {
	if req.RelatedBizNo == "" {
		return nil, domain.Keyed(domain.CodeReverseNeedsRelated, domain.KeyReverseNeedsRelated)
	}
	orig, err := s.entries.ListByBizNo(ctx, req.TenantID, req.RelatedBizNo)
	if err != nil {
		return nil, err
	}
	if len(orig) == 0 {
		return nil, domain.ErrNotFound
	}
	if orig[0].JournalID != "" {
		js, err := s.entries.ListByJournal(ctx, orig[0].JournalID)
		if err != nil {
			return nil, err
		}
		if len(js) > 0 {
			orig = js
		}
	}
	for _, e := range orig {
		if e.Command == domain.CmdReverse {
			return nil, domain.Keyed(domain.CodeReverseAlreadyReversed, domain.KeyReverseAlreadyReversed)
		}
	}
	if req.Holder.ID == "" {
		for _, e := range orig {
			if e.HolderType != domain.HolderSystemSubject {
				req.Holder = domain.Holder{Type: e.HolderType, ID: e.HolderID}
				req.AssetCode = e.AssetCode
				break
			}
		}
	}
	if req.Holder.ID != "" {
		ctx = domain.WithHolder(ctx, req.Holder.ID)
	}

	hash := requestHash(req)
	var result *domain.CommandResult
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		replay, err := s.checkIdempotency(ctx, req, hash)
		if err != nil {
			return err
		}
		if replay != nil {
			result = replay
			return nil
		}
		journalID := idgen.New("jnl_")
		if err := s.writeJournal(ctx, &domain.Journal{
			JournalID:    journalID,
			TenantID:     req.TenantID,
			BizNo:        req.BizNo,
			JournalType:  domain.JournalReverse,
			Status:       "posted",
			EntriesCount: len(orig),
			Ext:          orig[0].JournalID,
		}); err != nil {
			return err
		}
		var ids []string
		var last *domain.Account
		for _, e := range orig {
			acc, err := s.getOrOpenLocked(ctx, e.TenantID, domain.Holder{Type: e.HolderType, ID: e.HolderID}, e.AssetCode)
			if err != nil {
				return err
			}
			if err := AccountUsable(acc); err != nil {
				return err
			}
			revReq := req
			revReq.AssetCode = e.AssetCode
			revReq.Holder = domain.Holder{Type: e.HolderType, ID: e.HolderID}
			if e.Command == domain.CmdFreeze && e.FreezeID != "" {
				fz, err := s.freezes.GetByID(ctx, e.FreezeID)
				if err != nil {
					return err
				}
				if fz.Status != domain.FreezeFrozen {
					continue
				}
				if acc.Frozen < fz.Amount {
					return domain.Keyed(domain.CodeFreezeLedgerMismatch, domain.KeyFreezeLedgerMismatch)
				}
				acc.Frozen -= fz.Amount
				acc.Available += fz.Amount
				if err := s.accs.UpdateBalances(ctx, acc); err != nil {
					return err
				}
				if err := s.freezes.UpdateStatus(ctx, fz.FreezeID, domain.FreezeFrozen, domain.FreezeReleased); err != nil {
					return err
				}
				revReq.BizType = "unfreeze"
				entry, err := s.writeEntry(ctx, revReq, acc, domain.DirIN, fz.Amount, fz.FreezeID, journalID)
				if err != nil {
					return err
				}
				ids = append(ids, entry.EntryID)
				last = acc
				continue
			}
			if e.Direction == domain.DirIN {
				if acc.Available < e.Amount && !systemOverdraft(acc) {
					asset, err := s.assets.Get(ctx, acc.TenantID, acc.AssetCode)
					if err != nil {
						return err
					}
					if !asset.OverdraftAllowed {
						return domain.ErrInsufficient
					}
				}
				acc.Available -= e.Amount
				if err := s.accs.UpdateBalances(ctx, acc); err != nil {
					return err
				}
				entry, err := s.writeEntry(ctx, revReq, acc, domain.DirOUT, e.Amount, e.FreezeID, journalID)
				if err != nil {
					return err
				}
				ids = append(ids, entry.EntryID)
			} else {
				if e.Command == domain.CmdCapture && acc.Frozen >= 0 {
					acc.Available += e.Amount
				} else {
					acc.Available += e.Amount
				}
				if err := s.accs.UpdateBalances(ctx, acc); err != nil {
					return err
				}
				entry, err := s.writeEntry(ctx, revReq, acc, domain.DirIN, e.Amount, e.FreezeID, journalID)
				if err != nil {
					return err
				}
				ids = append(ids, entry.EntryID)
			}
			last = acc
		}
		if last == nil {
			return domain.ErrInvalidParam
		}
		res := newResult(last, ids, "", journalID)
		if err := s.saveIdempotency(ctx, req, hash, res); err != nil {
			return err
		}
		result = res
		return nil
	})
	return result, err
}
