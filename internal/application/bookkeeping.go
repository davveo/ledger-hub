package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

type Bookkeeping struct {
	tx        domain.TxManager
	assets    domain.AssetRepository
	accs      domain.AccountRepository
	entries   domain.EntryRepository
	freezes   domain.FreezeRepository
	idem      domain.IdempotencyRepository
	acl       *ACL
	journals  domain.JournalRepository
	fx        domain.FxRateRepository
	legs      domain.ExchangeLegRepository
	limiter   *Limiter
	sameShard func(a, b string) bool
}

func NewBookkeeping(
	tx domain.TxManager,
	assets domain.AssetRepository,
	accs domain.AccountRepository,
	entries domain.EntryRepository,
	freezes domain.FreezeRepository,
	idem domain.IdempotencyRepository,
	acl *ACL,
) *Bookkeeping {
	return &Bookkeeping{tx: tx, assets: assets, accs: accs, entries: entries, freezes: freezes, idem: idem, acl: acl}
}

func (s *Bookkeeping) UsePhase3(
	journals domain.JournalRepository,
	fx domain.FxRateRepository,
	legs domain.ExchangeLegRepository,
	limiter *Limiter,
	sameShard func(a, b string) bool,
) *Bookkeeping {
	s.journals = journals
	s.fx = fx
	s.legs = legs
	s.limiter = limiter
	s.sameShard = sameShard
	return s
}

func (s *Bookkeeping) Execute(ctx context.Context, req domain.CommandRequest) (*domain.CommandResult, error) {
	if req.Command == domain.CmdCapture || req.Command == domain.CmdRelease {
		if err := s.resolveFreezeHolder(ctx, &req); err != nil {
			return nil, err
		}
	}
	if req.Holder.ID != "" {
		ctx = domain.WithHolder(ctx, req.Holder.ID)
	}
	if err := validateBase(req); err != nil {
		return nil, err
	}
	if err := s.acl.Check(req); err != nil {
		return nil, err
	}
	switch req.Command {
	case domain.CmdCredit:
		return s.mutate(ctx, req, applyCredit)
	case domain.CmdDebit:
		return s.mutate(ctx, req, applyDebit)
	case domain.CmdFreeze:
		return s.mutate(ctx, req, applyFreeze)
	case domain.CmdCapture:
		return s.captureOrRelease(ctx, req, true)
	case domain.CmdRelease:
		return s.captureOrRelease(ctx, req, false)
	case domain.CmdTransfer:
		return s.transfer(ctx, req)
	case domain.CmdExchange:
		return s.exchange(ctx, req)
	case domain.CmdReverse:
		return s.reverse(ctx, req)
	default:
		return nil, domain.NewError(domain.CodeInvalidParam, "未知命令")
	}
}

type mutator func(ctx context.Context, s *Bookkeeping, req domain.CommandRequest, acc *domain.Account, asset *domain.Asset) (*domain.CommandResult, error)

func (s *Bookkeeping) mutate(ctx context.Context, req domain.CommandRequest, fn mutator) (*domain.CommandResult, error) {
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
		asset, err := s.assets.Get(ctx, req.TenantID, req.AssetCode)
		if err != nil {
			if domain.Is(err, domain.CodeNotFound) {
				return domain.ErrInvalidParam
			}
			return err
		}
		if asset.Status != domain.AssetActive {
			return domain.NewError(domain.CodeInvalidParam, "资产未启用")
		}
		if err := HolderAllowed(asset, req.Holder); err != nil {
			return err
		}
		acc, err := s.getOrOpenLocked(ctx, req.TenantID, req.Holder, req.AssetCode)
		if err != nil {
			return err
		}
		if err := AccountUsable(acc); err != nil {
			return err
		}
		if err := s.limiter.Check(ctx, req); err != nil {
			return err
		}
		res, err := fn(ctx, s, req, acc, asset)
		if err != nil {
			return err
		}
		if err := s.saveIdempotency(ctx, req, hash, res); err != nil {
			return err
		}
		result = res
		return nil
	})
	return result, err
}

func applyCredit(ctx context.Context, s *Bookkeeping, req domain.CommandRequest, acc *domain.Account, asset *domain.Asset) (*domain.CommandResult, error) {
	if req.Amount <= 0 {
		return nil, domain.ErrInvalidParam
	}
	journalID := idgen.New("jnl_")
	if err := s.writeJournal(ctx, &domain.Journal{
		JournalID: journalID, TenantID: req.TenantID, BizNo: req.BizNo,
		JournalType: domain.JournalPosting, Status: "posted", EntriesCount: 2,
	}); err != nil {
		return nil, err
	}
	acc.Available += req.Amount
	if err := s.accs.UpdateBalances(ctx, acc); err != nil {
		return nil, err
	}
	entry, err := s.writeEntry(ctx, req, acc, domain.DirIN, req.Amount, "", journalID)
	if err != nil {
		return nil, err
	}
	ids := []string{entry.EntryID}
	if cid, err := s.postSystem(ctx, req, domain.SystemPointIssuance, acc.AssetCode, domain.DirOUT, req.Amount, journalID); err != nil {
		return nil, err
	} else if cid != "" {
		ids = append(ids, cid)
	}
	_ = asset
	return newResult(acc, ids, "", journalID), nil
}

func applyDebit(ctx context.Context, s *Bookkeeping, req domain.CommandRequest, acc *domain.Account, asset *domain.Asset) (*domain.CommandResult, error) {
	if req.Amount <= 0 {
		return nil, domain.ErrInvalidParam
	}
	if acc.Available < req.Amount && !asset.OverdraftAllowed && !systemOverdraft(acc) {
		return nil, domain.ErrInsufficient
	}
	journalID := idgen.New("jnl_")
	if err := s.writeJournal(ctx, &domain.Journal{
		JournalID: journalID, TenantID: req.TenantID, BizNo: req.BizNo,
		JournalType: domain.JournalPosting, Status: "posted", EntriesCount: 2,
	}); err != nil {
		return nil, err
	}
	acc.Available -= req.Amount
	if err := s.accs.UpdateBalances(ctx, acc); err != nil {
		return nil, err
	}
	entry, err := s.writeEntry(ctx, req, acc, domain.DirOUT, req.Amount, "", journalID)
	if err != nil {
		return nil, err
	}
	ids := []string{entry.EntryID}
	if cid, err := s.postSystem(ctx, req, domain.SystemPointSink, acc.AssetCode, domain.DirIN, req.Amount, journalID); err != nil {
		return nil, err
	} else if cid != "" {
		ids = append(ids, cid)
	}
	return newResult(acc, ids, "", journalID), nil
}

func applyFreeze(ctx context.Context, s *Bookkeeping, req domain.CommandRequest, acc *domain.Account, asset *domain.Asset) (*domain.CommandResult, error) {
	if req.Amount <= 0 {
		return nil, domain.ErrInvalidParam
	}
	if !asset.FreezeSupported {
		return nil, domain.NewError(domain.CodeInvalidParam, "资产不支持冻结")
	}
	if acc.Available < req.Amount && !asset.OverdraftAllowed {
		return nil, domain.ErrInsufficient
	}
	acc.Available -= req.Amount
	acc.Frozen += req.Amount
	if err := s.accs.UpdateBalances(ctx, acc); err != nil {
		return nil, err
	}
	fz := &domain.FreezeOrder{
		FreezeID:  idgen.New("fz_"),
		BizNo:     req.BizNo,
		TenantID:  req.TenantID,
		AccountID: acc.AccountID,
		AssetCode: acc.AssetCode,
		Amount:    req.Amount,
		Status:    domain.FreezeFrozen,
		ExpireAt:  req.ExpireAt,
	}
	if err := s.freezes.Create(ctx, fz); err != nil {
		return nil, err
	}
	journalID := idgen.New("jnl_")
	if err := s.writeJournal(ctx, &domain.Journal{
		JournalID: journalID, TenantID: req.TenantID, BizNo: req.BizNo,
		JournalType: domain.JournalPosting, Status: "posted", EntriesCount: 1,
	}); err != nil {
		return nil, err
	}
	entry, err := s.writeEntry(ctx, req, acc, domain.DirOUT, req.Amount, fz.FreezeID, journalID)
	if err != nil {
		return nil, err
	}
	return newResult(acc, []string{entry.EntryID}, fz.FreezeID, journalID), nil
}

func (s *Bookkeeping) resolveFreezeHolder(ctx context.Context, req *domain.CommandRequest) error {
	if req.Holder.ID != "" && req.AssetCode != "" {
		return nil
	}
	var fz *domain.FreezeOrder
	var err error
	if req.FreezeID != "" {
		fz, err = s.freezes.GetByID(ctx, req.FreezeID)
	} else if req.RelatedBizNo != "" {
		fz, err = s.freezes.GetByBizNo(ctx, req.TenantID, req.RelatedBizNo)
	} else if req.BizNo != "" {
		fz, err = s.freezes.GetByBizNo(ctx, req.TenantID, req.BizNo)
	} else {
		return domain.ErrInvalidParam
	}
	if err != nil {
		return err
	}
	acc, err := s.accs.GetByID(ctx, fz.AccountID)
	if err != nil {
		return err
	}
	req.AssetCode = fz.AssetCode
	req.Holder = domain.Holder{Type: acc.HolderType, ID: acc.HolderID}
	if req.FreezeID == "" {
		req.FreezeID = fz.FreezeID
	}
	return nil
}

func (s *Bookkeeping) captureOrRelease(ctx context.Context, req domain.CommandRequest, capture bool) (*domain.CommandResult, error) {
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
		var fz *domain.FreezeOrder
		if req.FreezeID != "" {
			fz, err = s.freezes.GetByID(ctx, req.FreezeID)
		} else if req.RelatedBizNo != "" {
			fz, err = s.freezes.GetByBizNo(ctx, req.TenantID, req.RelatedBizNo)
		} else {
			fz, err = s.freezes.GetByBizNo(ctx, req.TenantID, req.BizNo)
		}
		if err != nil {
			return err
		}
		req.AssetCode = fz.AssetCode
		if fz.Status != domain.FreezeFrozen {
			return domain.ErrFreezeStateInvalid
		}
		acc, err := s.accs.GetByID(ctx, fz.AccountID)
		if err != nil {
			return err
		}
		locked, err := s.accs.GetForUpdate(ctx, acc.TenantID, domain.Holder{Type: acc.HolderType, ID: acc.HolderID}, acc.AssetCode)
		if err != nil {
			return err
		}
		if locked.Frozen < fz.Amount {
			return domain.NewError(domain.CodeInternal, "冻结余额与冻结单不一致")
		}
		if err := AccountUsable(locked); err != nil {
			return err
		}
		remain := fz.Amount
		capAmt := remain
		if capture && req.Amount > 0 {
			if req.Amount > remain {
				return domain.NewError(domain.CodeInvalidParam, "Capture 金额不能大于剩余冻结")
			}
			capAmt = req.Amount
		}
		locked.Frozen -= capAmt
		dir := domain.DirOUT
		if !capture {
			locked.Available += capAmt
			dir = domain.DirIN
		}
		if err := s.accs.UpdateBalances(ctx, locked); err != nil {
			return err
		}
		switch {
		case !capture:
			if err := s.freezes.UpdateStatus(ctx, fz.FreezeID, domain.FreezeFrozen, domain.FreezeReleased); err != nil {
				return err
			}
		case capAmt < remain:
			fz.Amount = remain - capAmt
			if err := s.freezes.Update(ctx, fz); err != nil {
				return err
			}
		default:
			if err := s.freezes.UpdateStatus(ctx, fz.FreezeID, domain.FreezeFrozen, domain.FreezeCaptured); err != nil {
				return err
			}
		}
		req.AssetCode = locked.AssetCode
		req.Holder = domain.Holder{Type: locked.HolderType, ID: locked.HolderID}
		req.Amount = capAmt
		journalID := idgen.New("jnl_")
		if err := s.writeJournal(ctx, &domain.Journal{
			JournalID: journalID, TenantID: req.TenantID, BizNo: req.BizNo,
			JournalType: domain.JournalPosting, Status: "posted", EntriesCount: 1,
		}); err != nil {
			return err
		}
		entry, err := s.writeEntry(ctx, req, locked, dir, capAmt, fz.FreezeID, journalID)
		if err != nil {
			return err
		}
		res := newResult(locked, []string{entry.EntryID}, fz.FreezeID, journalID)
		if err := s.saveIdempotency(ctx, req, hash, res); err != nil {
			return err
		}
		result = res
		return nil
	})
	return result, err
}

func (s *Bookkeeping) transfer(ctx context.Context, req domain.CommandRequest) (*domain.CommandResult, error) {
	if req.ToHolder == nil || req.ToHolder.ID == "" {
		return nil, domain.NewError(domain.CodeInvalidParam, "Transfer 需要 to holder")
	}
	if req.ToAssetCode != "" && req.ToAssetCode != req.AssetCode {
		return nil, domain.NewError(domain.CodeInvalidParam, "Transfer 禁止跨币种，请使用 Exchange")
	}
	if req.Amount <= 0 {
		return nil, domain.ErrInvalidParam
	}
	if s.sameShard != nil && req.ToHolder != nil && !s.sameShard(req.Holder.ID, req.ToHolder.ID) {
		return s.crossShardTransfer(ctx, req)
	}
	return s.transferSameShard(ctx, req)
}

func (s *Bookkeeping) crossShardTransfer(ctx context.Context, req domain.CommandRequest) (*domain.CommandResult, error) {
	hash := requestHash(req)
	ctxFrom := domain.WithHolder(ctx, req.Holder.ID)
	var replay *domain.CommandResult
	if err := s.tx.WithinTx(ctxFrom, func(ctx context.Context) error {
		r, err := s.checkIdempotency(ctx, req, hash)
		replay = r
		return err
	}); err != nil {
		return nil, err
	}
	if replay != nil {
		return replay, nil
	}
	pending := domain.Holder{Type: domain.HolderSystemSubject, ID: domain.SystemPendingSettlement}
	outReq := req
	outReq.ToHolder = &pending
	outReq.BizNo = req.BizNo + ":xshard:out"
	outReq.RelatedBizNo = req.BizNo
	res1, err := s.transferSameShard(ctxFrom, outReq)
	if err != nil {
		return nil, err
	}
	ctxTo := domain.WithHolder(ctx, req.ToHolder.ID)
	inReq := req
	inReq.Holder = pending
	inReq.BizNo = req.BizNo + ":xshard:in"
	inReq.RelatedBizNo = req.BizNo
	res2, err := s.transferSameShard(ctxTo, inReq)
	if err != nil {
		_, _ = s.reverse(ctxFrom, domain.CommandRequest{
			Command:      domain.CmdReverse,
			TenantID:     req.TenantID,
			SourceSystem: req.SourceSystem,
			BizType:      "xshard_rollback",
			BizNo:        req.BizNo + ":xshard:rollback",
			RelatedBizNo: outReq.BizNo,
			Holder:       req.Holder,
			AssetCode:    req.AssetCode,
		})
		return nil, err
	}
	res := &domain.CommandResult{
		Accepted:  true,
		JournalID: res2.JournalID,
		EntryIDs:  append(append([]string{}, res1.EntryIDs...), res2.EntryIDs...),
		Account:   res1.Account,
		ToAccount: res2.ToAccount,
	}
	_ = s.tx.WithinTx(ctxFrom, func(ctx context.Context) error {
		return s.saveIdempotency(ctx, req, hash, res)
	})
	return res, nil
}

func (s *Bookkeeping) transferSameShard(ctx context.Context, req domain.CommandRequest) (*domain.CommandResult, error) {
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
		asset, err := s.assets.Get(ctx, req.TenantID, req.AssetCode)
		if err != nil {
			if domain.Is(err, domain.CodeNotFound) {
				return domain.ErrInvalidParam
			}
			return err
		}
		if asset.Status != domain.AssetActive {
			return domain.NewError(domain.CodeInvalidParam, "资产未启用")
		}
		if err := HolderAllowed(asset, req.Holder); err != nil {
			return err
		}
		if err := HolderAllowed(asset, *req.ToHolder); err != nil {
			return err
		}
		fromKey := fmt.Sprintf("%s:%s", req.Holder.Type, req.Holder.ID)
		toKey := fmt.Sprintf("%s:%s", req.ToHolder.Type, req.ToHolder.ID)
		first, second := req.Holder, *req.ToHolder
		if fromKey > toKey {
			first, second = *req.ToHolder, req.Holder
		}
		accFirst, err := s.getOrOpenLocked(ctx, req.TenantID, first, req.AssetCode)
		if err != nil {
			return err
		}
		accSecond, err := s.getOrOpenLocked(ctx, req.TenantID, second, req.AssetCode)
		if err != nil {
			return err
		}
		var fromAcc, toAcc *domain.Account
		if accFirst.HolderID == req.Holder.ID && accFirst.HolderType == req.Holder.Type {
			fromAcc, toAcc = accFirst, accSecond
		} else {
			fromAcc, toAcc = accSecond, accFirst
		}
		if err := AccountUsable(fromAcc); err != nil {
			return err
		}
		if err := AccountUsable(toAcc); err != nil {
			return err
		}
		if fromAcc.Available < req.Amount && !asset.OverdraftAllowed && !systemOverdraft(fromAcc) {
			return domain.ErrInsufficient
		}
		fromAcc.Available -= req.Amount
		toAcc.Available += req.Amount
		if err := s.accs.UpdateBalances(ctx, fromAcc); err != nil {
			return err
		}
		if err := s.accs.UpdateBalances(ctx, toAcc); err != nil {
			return err
		}
		journalID := idgen.New("jnl_")
		if err := s.writeJournal(ctx, &domain.Journal{
			JournalID:    journalID,
			TenantID:     req.TenantID,
			BizNo:        req.BizNo,
			JournalType:  domain.JournalTransfer,
			Status:       "posted",
			EntriesCount: 2,
		}); err != nil {
			return err
		}
		e1, err := s.writeEntry(ctx, req, fromAcc, domain.DirOUT, req.Amount, "", journalID)
		if err != nil {
			return err
		}
		e2, err := s.writeEntry(ctx, req, toAcc, domain.DirIN, req.Amount, "", journalID)
		if err != nil {
			return err
		}
		res := newResult(fromAcc, []string{e1.EntryID, e2.EntryID}, "", journalID)
		res.ToAccount = toBalance(toAcc)
		if err := s.saveIdempotency(ctx, req, hash, res); err != nil {
			return err
		}
		result = res
		return nil
	})
	return result, err
}

func (s *Bookkeeping) getOrOpenLocked(ctx context.Context, tenantID string, holder domain.Holder, assetCode string) (*domain.Account, error) {
	asset, err := s.assets.Get(ctx, tenantID, assetCode)
	if err != nil {
		if domain.Is(err, domain.CodeNotFound) {
			return nil, domain.ErrInvalidParam
		}
		return nil, err
	}
	if asset.Status != domain.AssetActive {
		return nil, domain.NewError(domain.CodeInvalidParam, "资产未启用")
	}
	if err := HolderAllowed(asset, holder); err != nil {
		return nil, err
	}
	acc, err := s.accs.GetForUpdate(ctx, tenantID, holder, assetCode)
	if err == nil {
		if err := AccountUsable(acc); err != nil {
			return nil, err
		}
		return acc, nil
	}
	if !domain.Is(err, domain.CodeNotFound) {
		return nil, err
	}
	acc = &domain.Account{
		AccountID:  idgen.New("acc_"),
		TenantID:   tenantID,
		HolderType: holder.Type,
		HolderID:   holder.ID,
		AssetCode:  assetCode,
		Status:     domain.AccountActive,
		Version:    1,
	}
	if err := s.accs.Create(ctx, acc); err != nil {
		return nil, err
	}
	return s.accs.GetForUpdate(ctx, tenantID, holder, assetCode)
}

func (s *Bookkeeping) postSystem(ctx context.Context, req domain.CommandRequest, systemID, assetCode string, dir domain.Direction, amount int64, journalID string) (string, error) {
	acc, err := s.getOrOpenLocked(ctx, req.TenantID, domain.Holder{Type: domain.HolderSystemSubject, ID: systemID}, assetCode)
	if err != nil {
		return "", err
	}
	if dir == domain.DirOUT {
		acc.Available -= amount
	} else {
		acc.Available += amount
	}
	if err := s.accs.UpdateBalances(ctx, acc); err != nil {
		return "", err
	}
	e, err := s.writeEntry(ctx, req, acc, dir, amount, "", journalID)
	if err != nil {
		return "", err
	}
	return e.EntryID, nil
}

func (s *Bookkeeping) writeEntry(ctx context.Context, req domain.CommandRequest, acc *domain.Account, dir domain.Direction, amount int64, freezeID, journalID string) (*domain.LedgerEntry, error) {
	e := &domain.LedgerEntry{
		EntryID:        idgen.New("le_"),
		AccountID:      acc.AccountID,
		TenantID:       acc.TenantID,
		AssetCode:      acc.AssetCode,
		HolderType:     acc.HolderType,
		HolderID:       acc.HolderID,
		Direction:      dir,
		Amount:         amount,
		AvailableAfter: acc.Available,
		FrozenAfter:    acc.Frozen,
		Command:        req.Command,
		SourceSystem:   req.SourceSystem,
		BizType:        req.BizType,
		BizNo:          req.BizNo,
		JournalID:      journalID,
		FreezeID:       freezeID,
		RelatedBizNo:   req.RelatedBizNo,
	}
	if err := s.entries.Create(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Bookkeeping) checkIdempotency(ctx context.Context, req domain.CommandRequest, hash string) (*domain.CommandResult, error) {
	rec, err := s.idem.Get(ctx, req.TenantID, req.SourceSystem, req.BizNo, req.Command)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	if rec.RequestHash != hash {
		return nil, domain.ErrIdempotencyConflict
	}
	var res domain.CommandResult
	if err := json.Unmarshal([]byte(rec.ResponseJSON), &res); err != nil {
		return nil, err
	}
	res.IdempotentReplay = true
	return &res, nil
}

func (s *Bookkeeping) saveIdempotency(ctx context.Context, req domain.CommandRequest, hash string, res *domain.CommandResult) error {
	raw, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return s.idem.Create(ctx, &domain.IdempotencyRecord{
		TenantID:     req.TenantID,
		SourceSystem: req.SourceSystem,
		BizNo:        req.BizNo,
		Command:      req.Command,
		RequestHash:  hash,
		ResponseJSON: string(raw),
	})
}

func (s *Bookkeeping) writeJournal(ctx context.Context, j *domain.Journal) error {
	if s.journals == nil {
		return nil
	}
	if j.Status == "" {
		j.Status = "posted"
	}
	return s.journals.Create(ctx, j)
}

func validateBase(req domain.CommandRequest) error {
	if req.BizNo == "" || req.SourceSystem == "" || req.TenantID == "" {
		return domain.ErrInvalidParam
	}
	if req.Command == domain.CmdReverse {
		if req.RelatedBizNo == "" {
			return domain.NewError(domain.CodeInvalidParam, "Reverse 需要 related_biz_no")
		}
		return nil
	}
	if req.Command != domain.CmdCapture && req.Command != domain.CmdRelease {
		if req.Holder.ID == "" || req.AssetCode == "" {
			return domain.ErrInvalidParam
		}
	}
	return nil
}

func requestHash(req domain.CommandRequest) string {
	rate := ""
	if req.Fx != nil {
		rate = req.Fx.RateID + "|" + req.Fx.Rate
	}
	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%d|%s|%s|%d|%s|%d|%s|%d|%s",
		req.Command, req.TenantID, req.SourceSystem, req.BizNo,
		req.Holder.Type, req.Holder.ID, req.AssetCode, req.Amount,
		req.FreezeID, req.ToAssetCode, req.ToAmount, req.FeeAsset, req.FeeAmount, rate, req.Tolerance, req.RelatedBizNo)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func newResult(acc *domain.Account, entryIDs []string, freezeID, journalID string) *domain.CommandResult {
	return &domain.CommandResult{
		Accepted: true,
		FreezeID: freezeID,
		JournalID: journalID,
		EntryIDs: entryIDs,
		Account:  toBalance(acc),
	}
}

func toBalance(acc *domain.Account) *domain.Balance {
	return &domain.Balance{
		AccountID: acc.AccountID,
		Available: strconv.FormatInt(acc.Available, 10),
		Frozen:    strconv.FormatInt(acc.Frozen, 10),
	}
}
