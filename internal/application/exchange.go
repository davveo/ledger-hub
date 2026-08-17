package application

import (
	"context"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

func (s *Bookkeeping) exchange(ctx context.Context, req domain.CommandRequest) (*domain.CommandResult, error) {
	if req.ToAssetCode == "" || req.ToAssetCode == req.AssetCode {
		return nil, domain.NewError(domain.CodeInvalidParam, "Exchange 需要不同的 to.asset_code")
	}
	if req.Amount <= 0 {
		return nil, domain.ErrInvalidParam
	}
	if req.FeeAsset == "" {
		req.FeeAsset = req.AssetCode
	}
	if req.FeeAmount < 0 {
		return nil, domain.ErrInvalidParam
	}

	hash := requestHash(req)
	var result *domain.CommandResult
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		replay, err := s.checkIdempotency(ctx, req, hash)
		if err != nil {
			return err
		}
		if replay != nil {
			result = replay
			return nil
		}
		if err := s.limiter.Check(ctx, req); err != nil {
			return err
		}

		fromAsset, err := s.requireAsset(ctx, req.TenantID, req.AssetCode)
		if err != nil {
			return err
		}
		toAsset, err := s.requireAsset(ctx, req.TenantID, req.ToAssetCode)
		if err != nil {
			return err
		}

		quote, err := s.resolveQuote(ctx, req, fromAsset, toAsset)
		if err != nil {
			return err
		}
		expected, err := ExpectedToAmount(req.Amount, fromAsset.Precision, toAsset.Precision, quote.Rate)
		if err != nil {
			return err
		}
		if req.ToAmount <= 0 {
			req.ToAmount = expected
		}
		if req.ToAmount <= 0 {
			return domain.NewError(domain.CodeInvalidParam, "兑换目标金额无效")
		}
		if !WithinTolerance(expected, req.ToAmount, req.Tolerance) {
			return domain.ErrSlippage
		}

		firstCode, secondCode := req.AssetCode, req.ToAssetCode
		if firstCode > secondCode {
			firstCode, secondCode = secondCode, firstCode
		}
		accFirst, err := s.getOrOpenLocked(ctx, req.TenantID, req.Holder, firstCode)
		if err != nil {
			return err
		}
		accSecond, err := s.getOrOpenLocked(ctx, req.TenantID, req.Holder, secondCode)
		if err != nil {
			return err
		}
		fromAcc, toAcc := accFirst, accSecond
		if accFirst.AssetCode != req.AssetCode {
			fromAcc, toAcc = accSecond, accFirst
		}

		need := req.Amount
		if req.FeeAsset == req.AssetCode {
			need += req.FeeAmount
		}
		if fromAcc.Available < need && !fromAsset.OverdraftAllowed {
			return domain.ErrInsufficient
		}

		journalID := idgen.New("jnl_")
		if err := s.writeJournal(ctx, &domain.Journal{
			JournalID:    journalID,
			TenantID:     req.TenantID,
			BizNo:        req.BizNo,
			JournalType:  domain.JournalExchange,
			Status:       "posted",
			FxRateID:     quote.RateID,
			EntriesCount: 0,
		}); err != nil {
			return err
		}

		var ids []string
		fromAcc.Available -= req.Amount
		if err := s.accs.UpdateBalances(ctx, fromAcc); err != nil {
			return err
		}
		eFrom, err := s.writeEntry(ctx, req, fromAcc, domain.DirOUT, req.Amount, "", journalID)
		if err != nil {
			return err
		}
		ids = append(ids, eFrom.EntryID)

		if req.FeeAmount > 0 {
			feeAcc := fromAcc
			if req.FeeAsset != req.AssetCode {
				feeAsset, err := s.requireAsset(ctx, req.TenantID, req.FeeAsset)
				if err != nil {
					return err
				}
				feeAcc, err = s.getOrOpenLocked(ctx, req.TenantID, req.Holder, req.FeeAsset)
				if err != nil {
					return err
				}
				if feeAcc.Available < req.FeeAmount && !feeAsset.OverdraftAllowed {
					return domain.ErrInsufficient
				}
			}
			feeAcc.Available -= req.FeeAmount
			if err := s.accs.UpdateBalances(ctx, feeAcc); err != nil {
				return err
			}
			eFee, err := s.writeEntry(ctx, req, feeAcc, domain.DirOUT, req.FeeAmount, "", journalID)
			if err != nil {
				return err
			}
			ids = append(ids, eFee.EntryID)

			income, err := s.getOrOpenLocked(ctx, req.TenantID, domain.Holder{Type: domain.HolderSystemSubject, ID: domain.SystemFxFee}, req.FeeAsset)
			if err != nil {
				return err
			}
			income.Available += req.FeeAmount
			if err := s.accs.UpdateBalances(ctx, income); err != nil {
				return err
			}
			eInc, err := s.writeEntry(ctx, req, income, domain.DirIN, req.FeeAmount, "", journalID)
			if err != nil {
				return err
			}
			ids = append(ids, eInc.EntryID)
		}

		toAcc.Available += req.ToAmount
		if err := s.accs.UpdateBalances(ctx, toAcc); err != nil {
			return err
		}
		eTo, err := s.writeEntry(ctx, req, toAcc, domain.DirIN, req.ToAmount, "", journalID)
		if err != nil {
			return err
		}
		ids = append(ids, eTo.EntryID)

		clrFrom, err := s.getOrOpenLocked(ctx, req.TenantID, domain.Holder{Type: domain.HolderSystemSubject, ID: domain.SystemFxClearing}, req.AssetCode)
		if err != nil {
			return err
		}
		clrFrom.Available += req.Amount
		if err := s.accs.UpdateBalances(ctx, clrFrom); err != nil {
			return err
		}
		eClrIn, err := s.writeEntry(ctx, req, clrFrom, domain.DirIN, req.Amount, "", journalID)
		if err != nil {
			return err
		}
		ids = append(ids, eClrIn.EntryID)

		clrTo, err := s.getOrOpenLocked(ctx, req.TenantID, domain.Holder{Type: domain.HolderSystemSubject, ID: domain.SystemFxClearing}, req.ToAssetCode)
		if err != nil {
			return err
		}
		clrTo.Available -= req.ToAmount
		if err := s.accs.UpdateBalances(ctx, clrTo); err != nil {
			return err
		}
		eClrOut, err := s.writeEntry(ctx, req, clrTo, domain.DirOUT, req.ToAmount, "", journalID)
		if err != nil {
			return err
		}
		ids = append(ids, eClrOut.EntryID)

		if s.legs != nil {
			if err := s.legs.Create(ctx, &domain.ExchangeLeg{
				ExchangeID: idgen.New("ex_"),
				JournalID:  journalID,
				BizNo:      req.BizNo,
				TenantID:   req.TenantID,
				HolderType: req.Holder.Type,
				HolderID:   req.Holder.ID,
				FromAsset:  req.AssetCode,
				FromAmount: req.Amount,
				ToAsset:    req.ToAssetCode,
				ToAmount:   req.ToAmount,
				FeeAsset:   req.FeeAsset,
				FeeAmount:  req.FeeAmount,
				RateID:     quote.RateID,
				Rate:       quote.Rate,
				Status:     "posted",
			}); err != nil {
				return err
			}
		}

		res := newResult(fromAcc, ids, "", journalID)
		res.ToAccount = toBalance(toAcc)
		if err := s.saveIdempotency(ctx, req, hash, res); err != nil {
			return err
		}
		result = res
		return nil
	})
	return result, err
}

func (s *Bookkeeping) requireAsset(ctx context.Context, tenantID, code string) (*domain.Asset, error) {
	a, err := s.assets.Get(ctx, tenantID, code)
	if err != nil {
		if domain.Is(err, domain.CodeNotFound) {
			return nil, domain.ErrInvalidParam
		}
		return nil, err
	}
	if a.Status != domain.AssetActive {
		return nil, domain.NewError(domain.CodeInvalidParam, "资产未启用")
	}
	return a, nil
}

func (s *Bookkeeping) resolveQuote(ctx context.Context, req domain.CommandRequest, from, to *domain.Asset) (*domain.FxQuote, error) {
	q := req.Fx
	if q == nil {
		q = &domain.FxQuote{}
	}
	if q.BaseAsset == "" {
		q.BaseAsset = from.AssetCode
	}
	if q.QuoteAsset == "" {
		q.QuoteAsset = to.AssetCode
	}
	if q.Rate == "" && q.RateID != "" && s.fx != nil {
		snap, err := s.fx.Get(ctx, q.RateID)
		if err != nil {
			return nil, err
		}
		if snap.TenantID != req.TenantID {
			return nil, domain.ErrNotFound
		}
		q.Rate = snap.Rate
		q.BaseAsset = snap.BaseAsset
		q.QuoteAsset = snap.QuoteAsset
		q.RateSource = snap.RateSource
		q.QuotedAt = snap.QuotedAt
	}
	if q.Rate == "" && s.fx != nil {
		at := time.Now().UTC()
		if req.Fx != nil && !req.Fx.QuotedAt.IsZero() {
			at = req.Fx.QuotedAt
		}
		snap, err := s.fx.Find(ctx, req.TenantID, from.AssetCode, to.AssetCode, at)
		if err != nil {
			return nil, domain.NewError(domain.CodeInvalidParam, "缺少可用汇率快照")
		}
		q.RateID = snap.RateID
		q.Rate = snap.Rate
		q.RateSource = snap.RateSource
		q.QuotedAt = snap.QuotedAt
	}
	if q.Rate == "" {
		return nil, domain.NewError(domain.CodeInvalidParam, "Exchange 需要汇率")
	}
	return q, nil
}
