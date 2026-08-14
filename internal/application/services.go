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
	if _, err := s.assets.Get(ctx, tenantID, assetCode); err != nil {
		if domain.Is(err, domain.CodeNotFound) {
			return nil, domain.ErrInvalidParam
		}
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

func (s *AccountService) GetByID(ctx context.Context, accountID string) (*domain.Account, error) {
	return s.accs.GetByID(ctx, accountID)
}

type QueryService struct {
	entries domain.EntryRepository
	freezes domain.FreezeRepository
}

func NewQueryService(entries domain.EntryRepository, freezes domain.FreezeRepository) *QueryService {
	return &QueryService{entries: entries, freezes: freezes}
}

func (s *QueryService) EntriesByBizNo(ctx context.Context, tenantID, bizNo string) ([]*domain.LedgerEntry, error) {
	if bizNo == "" {
		return nil, domain.ErrInvalidParam
	}
	return s.entries.ListByBizNo(ctx, tenantID, bizNo)
}

func (s *QueryService) EntriesByHolder(ctx context.Context, tenantID string, holder domain.Holder, assetCode string, from, to *time.Time) ([]*domain.LedgerEntry, error) {
	if holder.ID == "" {
		return nil, domain.ErrInvalidParam
	}
	return s.entries.ListByHolder(ctx, tenantID, holder, assetCode, from, to)
}

func (s *QueryService) FreezeByID(ctx context.Context, freezeID string) (*domain.FreezeOrder, error) {
	return s.freezes.GetByID(ctx, freezeID)
}

func (s *QueryService) FreezeByBizNo(ctx context.Context, tenantID, bizNo string) (*domain.FreezeOrder, error) {
	return s.freezes.GetByBizNo(ctx, tenantID, bizNo)
}
