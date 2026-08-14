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
	tx      domain.TxManager
	assets  domain.AssetRepository
	accs    domain.AccountRepository
	entries domain.EntryRepository
	freezes domain.FreezeRepository
	idem    domain.IdempotencyRepository
	acl     *ACL
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

func (s *Bookkeeping) Execute(ctx context.Context, req domain.CommandRequest) (*domain.CommandResult, error) {
	if err := validateBase(req); err != nil {
		return nil, err
	}
	if req.Command != domain.CmdCapture && req.Command != domain.CmdRelease {
		if err := s.acl.Check(req); err != nil {
			return nil, err
		}
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
		return nil, domain.ErrNotImplemented
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
		acc, err := s.getOrOpenLocked(ctx, req.TenantID, req.Holder, req.AssetCode)
		if err != nil {
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

func applyCredit(ctx context.Context, s *Bookkeeping, req domain.CommandRequest, acc *domain.Account, _ *domain.Asset) (*domain.CommandResult, error) {
	if req.Amount <= 0 {
		return nil, domain.ErrInvalidParam
	}
	acc.Available += req.Amount
	if err := s.accs.UpdateBalances(ctx, acc); err != nil {
		return nil, err
	}
	entry, err := s.writeEntry(ctx, req, acc, domain.DirIN, req.Amount, "", "")
	if err != nil {
		return nil, err
	}
	return newResult(acc, []string{entry.EntryID}, "", ""), nil
}

func applyDebit(ctx context.Context, s *Bookkeeping, req domain.CommandRequest, acc *domain.Account, asset *domain.Asset) (*domain.CommandResult, error) {
	if req.Amount <= 0 {
		return nil, domain.ErrInvalidParam
	}
	if acc.Available < req.Amount && !asset.OverdraftAllowed {
		return nil, domain.ErrInsufficient
	}
	acc.Available -= req.Amount
	if err := s.accs.UpdateBalances(ctx, acc); err != nil {
		return nil, err
	}
	entry, err := s.writeEntry(ctx, req, acc, domain.DirOUT, req.Amount, "", "")
	if err != nil {
		return nil, err
	}
	return newResult(acc, []string{entry.EntryID}, "", ""), nil
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
	entry, err := s.writeEntry(ctx, req, acc, domain.DirOUT, req.Amount, fz.FreezeID, "")
	if err != nil {
		return nil, err
	}
	return newResult(acc, []string{entry.EntryID}, fz.FreezeID, ""), nil
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
		if err := s.acl.Check(req); err != nil {
			return err
		}
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
		locked.Frozen -= fz.Amount
		target := domain.FreezeCaptured
		dir := domain.DirOUT
		if !capture {
			locked.Available += fz.Amount
			target = domain.FreezeReleased
			dir = domain.DirIN
		}
		if err := s.accs.UpdateBalances(ctx, locked); err != nil {
			return err
		}
		if err := s.freezes.UpdateStatus(ctx, fz.FreezeID, domain.FreezeFrozen, target); err != nil {
			return err
		}
		req.AssetCode = locked.AssetCode
		req.Holder = domain.Holder{Type: locked.HolderType, ID: locked.HolderID}
		req.Amount = fz.Amount
		entry, err := s.writeEntry(ctx, req, locked, dir, fz.Amount, fz.FreezeID, "")
		if err != nil {
			return err
		}
		res := newResult(locked, []string{entry.EntryID}, fz.FreezeID, "")
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
		if fromAcc.Available < req.Amount && !asset.OverdraftAllowed {
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
	acc, err := s.accs.GetForUpdate(ctx, tenantID, holder, assetCode)
	if err == nil {
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

func validateBase(req domain.CommandRequest) error {
	if req.BizNo == "" || req.SourceSystem == "" || req.TenantID == "" {
		return domain.ErrInvalidParam
	}
	if req.Command != domain.CmdCapture && req.Command != domain.CmdRelease {
		if req.Holder.ID == "" || req.AssetCode == "" {
			return domain.ErrInvalidParam
		}
	}
	return nil
}

func requestHash(req domain.CommandRequest) string {
	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%d|%s|%s|%d",
		req.Command, req.TenantID, req.SourceSystem, req.BizNo,
		req.Holder.Type, req.Holder.ID, req.AssetCode, req.Amount,
		req.FreezeID, req.ToAssetCode, req.ToAmount)
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
