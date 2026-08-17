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

type AccountRepo struct {
	db      *gorm.DB
	cluster *Cluster
}

func NewAccountRepo(db *gorm.DB) *AccountRepo { return &AccountRepo{db: db} }

func (r *AccountRepo) WithCluster(c *Cluster) *AccountRepo {
	r.cluster = c
	return r
}

func (r *AccountRepo) GetByID(ctx context.Context, accountID string) (*domain.Account, error) {
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var m LedgerAccount
		err := db.Where("account_id = ?", accountID).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return m.toDomain(), nil
	}
	return nil, domain.ErrNotFound
}

func (r *AccountRepo) GetByIDInTenant(ctx context.Context, tenantID, accountID string) (*domain.Account, error) {
	if tenantID == "" || accountID == "" {
		return nil, domain.ErrInvalidParam
	}
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var m LedgerAccount
		err := db.Where("account_id = ? AND tenant_id = ?", accountID, tenantID).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return m.toDomain(), nil
	}
	return nil, domain.ErrNotFound
}

func (r *AccountRepo) Get(ctx context.Context, tenantID string, holder domain.Holder, assetCode string) (*domain.Account, error) {
	var m LedgerAccount
	err := routeDB(ctx, r.cluster, r.db, holder.ID).Where(
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
	err := routeDB(ctx, r.cluster, r.db, holder.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
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
	return routeDB(ctx, r.cluster, r.db, a.HolderID).Create(m).Error
}

func (r *AccountRepo) UpdateBalances(ctx context.Context, a *domain.Account) error {
	res := routeDB(ctx, r.cluster, r.db, a.HolderID).Model(&LedgerAccount{}).
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
		return domain.Keyed(domain.CodeOptimisticLock, domain.KeyOptimisticLock)
	}
	a.Version++
	return nil
}

func (r *AccountRepo) UpdateStatus(ctx context.Context, a *domain.Account) error {
	if a == nil || a.AccountID == "" {
		return domain.ErrInvalidParam
	}
	res := routeDB(ctx, r.cluster, r.db, a.HolderID).Model(&LedgerAccount{}).
		Where("account_id = ?", a.AccountID).
		Update("status", string(a.Status))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *AccountRepo) ListByTenant(ctx context.Context, tenantID, assetCode string) ([]*domain.Account, error) {
	var out []*domain.Account
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		q := db.Where("tenant_id = ?", tenantID)
		if assetCode != "" {
			q = q.Where("asset_code = ?", assetCode)
		}
		var rows []LedgerAccount
		if err := q.Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			out = append(out, rows[i].toDomain())
		}
	}
	return out, nil
}

func (r *AccountRepo) ListByHolder(ctx context.Context, tenantID string, holder domain.Holder, assetCode string) ([]*domain.Account, error) {
	if holder.ID == "" {
		return nil, domain.ErrInvalidParam
	}
	q := routeDB(ctx, r.cluster, r.db, holder.ID).Where("tenant_id = ? AND holder_type = ? AND holder_id = ?", tenantID, holder.Type, holder.ID)
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

type EntryRepo struct {
	db      *gorm.DB
	cluster *Cluster
}

func NewEntryRepo(db *gorm.DB) *EntryRepo { return &EntryRepo{db: db} }

func (r *EntryRepo) WithCluster(c *Cluster) *EntryRepo {
	r.cluster = c
	return r
}

func (r *EntryRepo) Create(ctx context.Context, e *domain.LedgerEntry) error {
	return routeDB(ctx, r.cluster, r.db, e.HolderID).Create(entryFromDomain(e)).Error
}

func (r *EntryRepo) ListByBizNo(ctx context.Context, tenantID, bizNo string) ([]*domain.LedgerEntry, error) {
	var out []*domain.LedgerEntry
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var rows []LedgerEntry
		if err := db.Where("tenant_id = ? AND biz_no = ?", tenantID, bizNo).Order("id asc").Find(&rows).Error; err != nil {
			return nil, err
		}
		out = append(out, entriesToDomain(rows)...)
	}
	return out, nil
}

func (r *EntryRepo) ListByHolder(ctx context.Context, tenantID string, holder domain.Holder, assetCode string, from, to *time.Time, page domain.Page) ([]*domain.LedgerEntry, error) {
	page = page.Clamp(50, 200)
	q := routeDB(ctx, r.cluster, r.db, holder.ID).Where("tenant_id = ? AND holder_type = ? AND holder_id = ?", tenantID, holder.Type, holder.ID)
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
	if err := q.Order("id desc").Offset(page.Offset).Limit(page.Limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return entriesToDomain(rows), nil
}

func (r *EntryRepo) ListByRange(ctx context.Context, tenantID, sourceSystem, assetCode string, from, to time.Time) ([]*domain.LedgerEntry, error) {
	var out []*domain.LedgerEntry
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		q := db.Where("tenant_id = ? AND created_at >= ? AND created_at < ?", tenantID, from, to)
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
		out = append(out, entriesToDomain(rows)...)
	}
	return out, nil
}

func (r *EntryRepo) ListByAccount(ctx context.Context, accountID string) ([]*domain.LedgerEntry, error) {
	var out []*domain.LedgerEntry
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var rows []LedgerEntry
		if err := db.Where("account_id = ?", accountID).Order("id asc").Find(&rows).Error; err != nil {
			return nil, err
		}
		out = append(out, entriesToDomain(rows)...)
	}
	return out, nil
}

func (r *EntryRepo) ListByJournal(ctx context.Context, journalID string) ([]*domain.LedgerEntry, error) {
	if journalID == "" {
		return nil, domain.ErrInvalidParam
	}
	var out []*domain.LedgerEntry
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var rows []LedgerEntry
		if err := db.Where("journal_id = ?", journalID).Order("id asc").Find(&rows).Error; err != nil {
			return nil, err
		}
		out = append(out, entriesToDomain(rows)...)
	}
	return out, nil
}

func entriesToDomain(rows []LedgerEntry) []*domain.LedgerEntry {
	out := make([]*domain.LedgerEntry, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out
}

type FreezeRepo struct {
	db      *gorm.DB
	cluster *Cluster
}

func NewFreezeRepo(db *gorm.DB) *FreezeRepo { return &FreezeRepo{db: db} }

func (r *FreezeRepo) WithCluster(c *Cluster) *FreezeRepo {
	r.cluster = c
	return r
}

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
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var m LedgerFreeze
		err := db.Where("freeze_id = ?", freezeID).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return m.toDomain(), nil
	}
	return nil, domain.ErrNotFound
}

func (r *FreezeRepo) GetByIDInTenant(ctx context.Context, tenantID, freezeID string) (*domain.FreezeOrder, error) {
	if tenantID == "" || freezeID == "" {
		return nil, domain.ErrInvalidParam
	}
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var m LedgerFreeze
		err := db.Where("freeze_id = ? AND tenant_id = ?", freezeID, tenantID).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return m.toDomain(), nil
	}
	return nil, domain.ErrNotFound
}

func (r *FreezeRepo) GetByBizNo(ctx context.Context, tenantID, bizNo string) (*domain.FreezeOrder, error) {
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var m LedgerFreeze
		err := db.Where("tenant_id = ? AND biz_no = ?", tenantID, bizNo).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return m.toDomain(), nil
	}
	return nil, domain.ErrNotFound
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

func (r *FreezeRepo) Update(ctx context.Context, f *domain.FreezeOrder) error {
	res := dbFrom(ctx, r.db).Model(&LedgerFreeze{}).
		Where("freeze_id = ?", f.FreezeID).
		Updates(map[string]interface{}{
			"amount": f.Amount,
			"status": string(f.Status),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *FreezeRepo) ListExpired(ctx context.Context, now time.Time, limit int) ([]*domain.FreezeOrder, error) {
	var out []*domain.FreezeOrder
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var rows []LedgerFreeze
		q := db.Where("status = ? AND expire_at IS NOT NULL AND expire_at <= ?", domain.FreezeFrozen, now)
		if limit > 0 {
			q = q.Limit(limit)
		}
		if err := q.Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			out = append(out, rows[i].toDomain())
		}
	}
	return out, nil
}

func (r *FreezeRepo) ListFrozen(ctx context.Context, tenantID, assetCode string) ([]*domain.FreezeOrder, error) {
	var out []*domain.FreezeOrder
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		q := db.Where("tenant_id = ? AND status = ?", tenantID, domain.FreezeFrozen)
		if assetCode != "" {
			q = q.Where("asset_code = ?", assetCode)
		}
		var rows []LedgerFreeze
		if err := q.Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			out = append(out, rows[i].toDomain())
		}
	}
	return out, nil
}

func (r *FreezeRepo) ListByHolder(ctx context.Context, tenantID string, holder domain.Holder, assetCode, status string, page domain.Page) ([]*domain.FreezeOrder, error) {
	page = page.Clamp(50, 200)
	var accIDs []string
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		q := db.Model(&LedgerAccount{}).Where("tenant_id = ? AND holder_type = ? AND holder_id = ?", tenantID, holder.Type, holder.ID)
		if assetCode != "" {
			q = q.Where("asset_code = ?", assetCode)
		}
		var ids []string
		if err := q.Pluck("account_id", &ids).Error; err != nil {
			return nil, err
		}
		accIDs = append(accIDs, ids...)
	}
	if len(accIDs) == 0 {
		return nil, nil
	}
	var out []*domain.FreezeOrder
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		q := db.Where("tenant_id = ? AND account_id IN ?", tenantID, accIDs)
		if status != "" {
			q = q.Where("status = ?", status)
		}
		var rows []LedgerFreeze
		if err := q.Order("id desc").Offset(page.Offset).Limit(page.Limit).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			out = append(out, rows[i].toDomain())
		}
	}
	return out, nil
}

type IdempotencyRepo struct {
	db      *gorm.DB
	cluster *Cluster
}

func NewIdempotencyRepo(db *gorm.DB) *IdempotencyRepo { return &IdempotencyRepo{db: db} }

func (r *IdempotencyRepo) WithCluster(c *Cluster) *IdempotencyRepo {
	r.cluster = c
	return r
}

func (r *IdempotencyRepo) Get(ctx context.Context, tenantID, sourceSystem, bizNo string, cmd domain.Command) (*domain.IdempotencyRecord, error) {
	dbs := []*gorm.DB{routeDB(ctx, r.cluster, r.db, domain.HolderIDFrom(ctx))}
	if domain.HolderIDFrom(ctx) == "" && r.cluster != nil {
		dbs = scanDBs(ctx, r.cluster, r.db)
	}
	for _, db := range dbs {
		var m LedgerIdempotency
		err := db.Where(
			"tenant_id = ? AND source_system = ? AND biz_no = ? AND command = ?",
			tenantID, sourceSystem, bizNo, string(cmd),
		).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
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
	return nil, nil
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

func (r *IdempotencyRepo) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	var n int64
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		res := db.Where("created_at < ?", before).Delete(&LedgerIdempotency{})
		if res.Error != nil {
			return n, res.Error
		}
		n += res.RowsAffected
	}
	return n, nil
}
