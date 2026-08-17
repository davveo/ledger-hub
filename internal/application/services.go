package application

import (
	"context"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

type AssetService struct {
	assets domain.AssetRepository
}

func NewAssetService(assets domain.AssetRepository) *AssetService {
	return &AssetService{assets: assets}
}

func (s *AssetService) Save(ctx context.Context, a *domain.Asset) error {
	if a.TenantID == "" || a.AssetCode == "" || a.Name == "" {
		return domain.ErrInvalidParam
	}
	if a.Status == "" {
		a.Status = domain.AssetActive
	}
	if a.AssetClass == "" {
		a.AssetClass = "other"
	}
	return s.assets.Save(ctx, a)
}

func (s *AssetService) Get(ctx context.Context, tenantID, assetCode string) (*domain.Asset, error) {
	return s.assets.Get(ctx, tenantID, assetCode)
}

func (s *AssetService) List(ctx context.Context, tenantID string) ([]*domain.Asset, error) {
	return s.assets.List(ctx, tenantID)
}

func (s *AssetService) SetStatus(ctx context.Context, tenantID, assetCode string, status domain.AssetStatus) (*domain.Asset, error) {
	if status != domain.AssetActive && status != domain.AssetDisabled {
		return nil, domain.ErrInvalidParam
	}
	a, err := s.assets.Get(ctx, tenantID, assetCode)
	if err != nil {
		return nil, err
	}
	a.Status = status
	if err := s.assets.Save(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

type AccountService struct {
	assets domain.AssetRepository
	accs   domain.AccountRepository
}

func NewAccountService(assets domain.AssetRepository, accs domain.AccountRepository) *AccountService {
	return &AccountService{assets: assets, accs: accs}
}

func (s *AccountService) Open(ctx context.Context, tenantID string, holder domain.Holder, assetCode string) (*domain.Account, error) {
	if tenantID == "" || holder.ID == "" || assetCode == "" {
		return nil, domain.ErrInvalidParam
	}
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
	acc, err := s.accs.Get(ctx, tenantID, holder, assetCode)
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
		existing, getErr := s.accs.Get(ctx, tenantID, holder, assetCode)
		if getErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return acc, nil
}

func (s *AccountService) Get(ctx context.Context, tenantID string, holder domain.Holder, assetCode string) (*domain.Account, error) {
	return s.accs.Get(ctx, tenantID, holder, assetCode)
}

func (s *AccountService) GetByID(ctx context.Context, tenantID, accountID string) (*domain.Account, error) {
	acc, err := s.accs.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if err := tenantMatch(acc.TenantID, tenantID); err != nil {
		return nil, err
	}
	return acc, nil
}

func (s *AccountService) List(ctx context.Context, tenantID, assetCode string) ([]*domain.Account, error) {
	return s.accs.ListByTenant(ctx, tenantID, assetCode)
}

func (s *AccountService) ListByHolder(ctx context.Context, tenantID string, holder domain.Holder, assetCode string) ([]*domain.Account, error) {
	if holder.ID == "" {
		return nil, domain.ErrInvalidParam
	}
	if holder.Type == "" {
		holder.Type = domain.HolderUser
	}
	return s.accs.ListByHolder(ctx, tenantID, holder, assetCode)
}

func (s *AccountService) SetStatus(ctx context.Context, tenantID, accountID string, status domain.AccountStatus) (*domain.Account, error) {
	if status != domain.AccountActive && status != domain.AccountDisabled {
		return nil, domain.ErrInvalidParam
	}
	acc, err := s.GetByID(ctx, tenantID, accountID)
	if err != nil {
		return nil, err
	}
	acc.Status = status
	if err := s.accs.UpdateStatus(ctx, acc); err != nil {
		return nil, err
	}
	return acc, nil
}

type QueryService struct {
	entries  domain.EntryRepository
	freezes  domain.FreezeRepository
	journals domain.JournalRepository
}

func NewQueryService(entries domain.EntryRepository, freezes domain.FreezeRepository) *QueryService {
	return &QueryService{entries: entries, freezes: freezes}
}

func (s *QueryService) WithJournal(journals domain.JournalRepository) *QueryService {
	s.journals = journals
	return s
}

func (s *QueryService) EntriesByBizNo(ctx context.Context, tenantID, bizNo string) ([]*domain.LedgerEntry, error) {
	if bizNo == "" {
		return nil, domain.ErrInvalidParam
	}
	return s.entries.ListByBizNo(ctx, tenantID, bizNo)
}

func (s *QueryService) EntriesByHolder(ctx context.Context, tenantID string, holder domain.Holder, assetCode string, from, to *time.Time, page domain.Page) ([]*domain.LedgerEntry, error) {
	if holder.ID == "" {
		return nil, domain.ErrInvalidParam
	}
	if holder.Type == "" {
		holder.Type = domain.HolderUser
	}
	return s.entries.ListByHolder(ctx, tenantID, holder, assetCode, from, to, page.Clamp(50, 200))
}

func (s *QueryService) FreezeByID(ctx context.Context, tenantID, freezeID string) (*domain.FreezeOrder, error) {
	fz, err := s.freezes.GetByID(ctx, freezeID)
	if err != nil {
		return nil, err
	}
	if err := tenantMatch(fz.TenantID, tenantID); err != nil {
		return nil, err
	}
	return fz, nil
}

func (s *QueryService) FreezeByBizNo(ctx context.Context, tenantID, bizNo string) (*domain.FreezeOrder, error) {
	return s.freezes.GetByBizNo(ctx, tenantID, bizNo)
}

func (s *QueryService) FreezesByHolder(ctx context.Context, tenantID string, holder domain.Holder, assetCode, status string, page domain.Page) ([]*domain.FreezeOrder, error) {
	if holder.ID == "" {
		return nil, domain.ErrInvalidParam
	}
	if holder.Type == "" {
		holder.Type = domain.HolderUser
	}
	return s.freezes.ListByHolder(ctx, tenantID, holder, assetCode, status, page.Clamp(50, 200))
}

func (s *QueryService) EntriesByAccount(ctx context.Context, tenantID, accountID string) ([]*domain.LedgerEntry, error) {
	if accountID == "" {
		return nil, domain.ErrInvalidParam
	}
	list, err := s.entries.ListByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := list[:0]
	for _, e := range list {
		if e.TenantID == tenantID {
			out = append(out, e)
		}
	}
	if len(out) == 0 && len(list) > 0 {
		return nil, domain.ErrNotFound
	}
	return out, nil
}

func (s *QueryService) ExpiredFreezes(ctx context.Context, tenantID string, now time.Time, limit int) ([]*domain.FreezeOrder, error) {
	if s.freezes == nil {
		return nil, domain.ErrNotImplemented
	}
	if limit <= 0 {
		limit = 50
	}
	list, err := s.freezes.ListExpired(ctx, now, limit)
	if err != nil {
		return nil, err
	}
	if tenantID == "" {
		return list, nil
	}
	out := list[:0]
	for _, f := range list {
		if f.TenantID == tenantID {
			out = append(out, f)
		}
	}
	return out, nil
}

func (s *QueryService) Frozen(ctx context.Context, tenantID, assetCode string) ([]*domain.FreezeOrder, error) {
	if s.freezes == nil {
		return nil, domain.ErrNotImplemented
	}
	return s.freezes.ListFrozen(ctx, tenantID, assetCode)
}

func (s *QueryService) Journal(ctx context.Context, tenantID, journalID string) (*domain.Journal, []*domain.LedgerEntry, error) {
	if s.journals == nil {
		return nil, nil, domain.ErrNotImplemented
	}
	j, err := s.journals.Get(ctx, journalID)
	if err != nil {
		return nil, nil, err
	}
	if err := tenantMatch(j.TenantID, tenantID); err != nil {
		return nil, nil, err
	}
	entries, err := s.entries.ListByJournal(ctx, journalID)
	if err != nil {
		return nil, nil, err
	}
	return j, entries, nil
}
