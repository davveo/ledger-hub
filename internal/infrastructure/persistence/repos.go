package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/davveo/ledger-hub/internal/domain"
)

type AssetRepo struct{ db *gorm.DB }

func NewAssetRepo(db *gorm.DB) *AssetRepo { return &AssetRepo{db: db} }

func (r *AssetRepo) Save(ctx context.Context, a *domain.Asset) error {
	m := assetFromDomain(a)
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "asset_code"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "asset_class", "currency_code", "precision", "holder_types", "freeze_supported", "overdraft_allowed", "status", "ext", "updated_at"}),
	}).Create(m).Error
}

func (r *AssetRepo) Get(ctx context.Context, tenantID, assetCode string) (*domain.Asset, error) {
	var m LedgerAsset
	err := dbFrom(ctx, r.db).Where("tenant_id = ? AND asset_code = ?", tenantID, assetCode).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *AssetRepo) List(ctx context.Context, tenantID string) ([]*domain.Asset, error) {
	var rows []LedgerAsset
	if err := dbFrom(ctx, r.db).Where("tenant_id = ?", tenantID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.Asset, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

type AccountRepo struct{ db *gorm.DB }

func NewAccountRepo(db *gorm.DB) *AccountRepo { return &AccountRepo{db: db} }

func (r *AccountRepo) GetByID(ctx context.Context, accountID string) (*domain.Account, error) {
	var m LedgerAccount
	err := dbFrom(ctx, r.db).Where("account_id = ?", accountID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *AccountRepo) Get(ctx context.Context, tenantID string, holder domain.Holder, assetCode string) (*domain.Account, error) {
	var m LedgerAccount
	err := dbFrom(ctx, r.db).Where(
		"tenant_id = ? AND holder_type = ? AND holder_id = ? AND asset_code = ?",
		tenantID, holder.Type, holder.ID, assetCode,
	).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *AccountRepo) GetForUpdate(ctx context.Context, tenantID string, holder domain.Holder, assetCode string) (*domain.Account, error) {
	var m LedgerAccount
	err := dbFrom(ctx, r.db).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"tenant_id = ? AND holder_type = ? AND holder_id = ? AND asset_code = ?",
		tenantID, holder.Type, holder.ID, assetCode,
	).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *AccountRepo) Create(ctx context.Context, a *domain.Account) error {
	m := &LedgerAccount{
		AccountID:  a.AccountID,
		TenantID:   a.TenantID,
		HolderType: string(a.HolderType),
		HolderID:   a.HolderID,
		AssetCode:  a.AssetCode,
		Available:  a.Available,
		Frozen:     a.Frozen,
		Version:    a.Version,
		Status:     string(a.Status),
	}
	return dbFrom(ctx, r.db).Create(m).Error
}

func (r *AccountRepo) UpdateBalances(ctx context.Context, a *domain.Account) error {
	res := dbFrom(ctx, r.db).Model(&LedgerAccount{}).
		Where("account_id = ? AND version = ?", a.AccountID, a.Version).
		Updates(map[string]interface{}{
			"available": a.Available,
			"frozen":    a.Frozen,
			"version":   a.Version + 1,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.NewError(domain.CodeInternal, "账户乐观锁冲突，请重试")
	}
	a.Version++
	return nil
}

func (r *AccountRepo) ListByTenant(ctx context.Context, tenantID, assetCode string) ([]*domain.Account, error) {
	q := dbFrom(ctx, r.db).Where("tenant_id = ?", tenantID)
	if assetCode != "" {
		q = q.Where("asset_code = ?", assetCode)
	}
	var rows []LedgerAccount
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.Account, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

type EntryRepo struct{ db *gorm.DB }

func NewEntryRepo(db *gorm.DB) *EntryRepo { return &EntryRepo{db: db} }

func (r *EntryRepo) Create(ctx context.Context, e *domain.LedgerEntry) error {
	return dbFrom(ctx, r.db).Create(entryFromDomain(e)).Error
}

func (r *EntryRepo) ListByBizNo(ctx context.Context, tenantID, bizNo string) ([]*domain.LedgerEntry, error) {
	var rows []LedgerEntry
	if err := dbFrom(ctx, r.db).Where("tenant_id = ? AND biz_no = ?", tenantID, bizNo).Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return entriesToDomain(rows), nil
}

func (r *EntryRepo) ListByHolder(ctx context.Context, tenantID string, holder domain.Holder, assetCode string, from, to *time.Time) ([]*domain.LedgerEntry, error) {
	q := dbFrom(ctx, r.db).Where("tenant_id = ? AND holder_type = ? AND holder_id = ?", tenantID, holder.Type, holder.ID)
	if assetCode != "" {
		q = q.Where("asset_code = ?", assetCode)
	}
	if from != nil {
		q = q.Where("created_at >= ?", *from)
	}
	if to != nil {
		q = q.Where("created_at < ?", *to)
	}
	var rows []LedgerEntry
	if err := q.Order("id desc").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	return entriesToDomain(rows), nil
}

func (r *EntryRepo) ListByRange(ctx context.Context, tenantID, sourceSystem, assetCode string, from, to time.Time) ([]*domain.LedgerEntry, error) {
	q := dbFrom(ctx, r.db).Where("tenant_id = ? AND created_at >= ? AND created_at < ?", tenantID, from, to)
	if sourceSystem != "" {
		q = q.Where("source_system = ?", sourceSystem)
	}
	if assetCode != "" {
		q = q.Where("asset_code = ?", assetCode)
	}
	var rows []LedgerEntry
	if err := q.Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return entriesToDomain(rows), nil
}

func (r *EntryRepo) ListByAccount(ctx context.Context, accountID string) ([]*domain.LedgerEntry, error) {
	var rows []LedgerEntry
	if err := dbFrom(ctx, r.db).Where("account_id = ?", accountID).Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return entriesToDomain(rows), nil
}

func entriesToDomain(rows []LedgerEntry) []*domain.LedgerEntry {
	out := make([]*domain.LedgerEntry, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out
}

type FreezeRepo struct{ db *gorm.DB }

func NewFreezeRepo(db *gorm.DB) *FreezeRepo { return &FreezeRepo{db: db} }

func (r *FreezeRepo) Create(ctx context.Context, f *domain.FreezeOrder) error {
	m := &LedgerFreeze{
		FreezeID:  f.FreezeID,
		BizNo:     f.BizNo,
		TenantID:  f.TenantID,
		AccountID: f.AccountID,
		AssetCode: f.AssetCode,
		Amount:    f.Amount,
		Status:    string(f.Status),
		ExpireAt:  f.ExpireAt,
	}
	return dbFrom(ctx, r.db).Create(m).Error
}

func (r *FreezeRepo) GetByID(ctx context.Context, freezeID string) (*domain.FreezeOrder, error) {
	var m LedgerFreeze
	err := dbFrom(ctx, r.db).Where("freeze_id = ?", freezeID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *FreezeRepo) GetByBizNo(ctx context.Context, tenantID, bizNo string) (*domain.FreezeOrder, error) {
	var m LedgerFreeze
	err := dbFrom(ctx, r.db).Where("tenant_id = ? AND biz_no = ?", tenantID, bizNo).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *FreezeRepo) UpdateStatus(ctx context.Context, freezeID string, from, to domain.FreezeStatus) error {
	res := dbFrom(ctx, r.db).Model(&LedgerFreeze{}).
		Where("freeze_id = ? AND status = ?", freezeID, from).
		Update("status", string(to))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrFreezeStateInvalid
	}
	return nil
}

func (r *FreezeRepo) ListExpired(ctx context.Context, now time.Time, limit int) ([]*domain.FreezeOrder, error) {
	var rows []LedgerFreeze
	q := dbFrom(ctx, r.db).Where("status = ? AND expire_at IS NOT NULL AND expire_at <= ?", domain.FreezeFrozen, now)
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.FreezeOrder, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (r *FreezeRepo) ListFrozen(ctx context.Context, tenantID, assetCode string) ([]*domain.FreezeOrder, error) {
	q := dbFrom(ctx, r.db).Where("tenant_id = ? AND status = ?", tenantID, domain.FreezeFrozen)
	if assetCode != "" {
		q = q.Where("asset_code = ?", assetCode)
	}
	var rows []LedgerFreeze
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.FreezeOrder, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

type IdempotencyRepo struct{ db *gorm.DB }

func NewIdempotencyRepo(db *gorm.DB) *IdempotencyRepo { return &IdempotencyRepo{db: db} }

func (r *IdempotencyRepo) Get(ctx context.Context, tenantID, sourceSystem, bizNo string, cmd domain.Command) (*domain.IdempotencyRecord, error) {
	var m LedgerIdempotency
	err := dbFrom(ctx, r.db).Where(
		"tenant_id = ? AND source_system = ? AND biz_no = ? AND command = ?",
		tenantID, sourceSystem, bizNo, string(cmd),
	).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &domain.IdempotencyRecord{
		TenantID:     m.TenantID,
		SourceSystem: m.SourceSystem,
		BizNo:        m.BizNo,
		Command:      domain.Command(m.Command),
		RequestHash:  m.RequestHash,
		ResponseJSON: m.ResponseJSON,
	}, nil
}

func (r *IdempotencyRepo) Create(ctx context.Context, rec *domain.IdempotencyRecord) error {
	m := &LedgerIdempotency{
		TenantID:     rec.TenantID,
		SourceSystem: rec.SourceSystem,
		BizNo:        rec.BizNo,
		Command:      string(rec.Command),
		RequestHash:  rec.RequestHash,
		ResponseJSON: rec.ResponseJSON,
	}
	return dbFrom(ctx, r.db).Create(m).Error
}
