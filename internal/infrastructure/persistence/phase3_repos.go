package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/davveo/ledger-hub/internal/domain"
)

type JournalRepo struct {
	db      *gorm.DB
	cluster *Cluster
}

func NewJournalRepo(db *gorm.DB) *JournalRepo { return &JournalRepo{db: db} }

func (r *JournalRepo) WithCluster(c *Cluster) *JournalRepo {
	r.cluster = c
	return r
}

func (r *JournalRepo) Create(ctx context.Context, j *domain.Journal) error {
	m := &LedgerJournal{
		JournalID:    j.JournalID,
		TenantID:     j.TenantID,
		BizNo:        j.BizNo,
		JournalType:  j.JournalType,
		Status:       j.Status,
		EntriesCount: j.EntriesCount,
		FxRateID:     j.FxRateID,
		Ext:          j.Ext,
	}
	return dbFrom(ctx, r.db).Create(m).Error
}

func (r *JournalRepo) Get(ctx context.Context, journalID string) (*domain.Journal, error) {
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var m LedgerJournal
		err := db.Where("journal_id = ?", journalID).First(&m).Error
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

func (r *JournalRepo) ListByRange(ctx context.Context, tenantID, journalType string, from, to time.Time) ([]*domain.Journal, error) {
	var out []*domain.Journal
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		q := db.Where("tenant_id = ? AND created_at >= ? AND created_at < ?", tenantID, from, to)
		if journalType != "" {
			q = q.Where("journal_type = ?", journalType)
		}
		var rows []LedgerJournal
		if err := q.Order("id asc").Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			out = append(out, rows[i].toDomain())
		}
	}
	return out, nil
}

type FxRateRepo struct{ db *gorm.DB }

func NewFxRateRepo(db *gorm.DB) *FxRateRepo { return &FxRateRepo{db: db} }

func (r *FxRateRepo) Save(ctx context.Context, rate *domain.FxRate) error {
	m := &LedgerFxRate{
		RateID:     rate.RateID,
		TenantID:   rate.TenantID,
		BaseAsset:  rate.BaseAsset,
		QuoteAsset: rate.QuoteAsset,
		Rate:       rate.Rate,
		RateSource: rate.RateSource,
		ValidFrom:  rate.ValidFrom,
		ValidTo:    rate.ValidTo,
		QuotedAt:   rate.QuotedAt,
		CreatedBy:  rate.CreatedBy,
	}
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "rate_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"rate", "rate_source", "valid_from", "valid_to", "quoted_at", "created_by"}),
	}).Create(m).Error
}

func (r *FxRateRepo) Get(ctx context.Context, rateID string) (*domain.FxRate, error) {
	var m LedgerFxRate
	err := dbFrom(ctx, r.db).Where("rate_id = ?", rateID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *FxRateRepo) Find(ctx context.Context, tenantID, base, quote string, at time.Time) (*domain.FxRate, error) {
	q := dbFrom(ctx, r.db).Where("tenant_id = ? AND base_asset = ? AND quote_asset = ?", tenantID, base, quote)
	if !at.IsZero() {
		q = q.Where("quoted_at <= ?", at).
			Where("(valid_from IS NULL OR valid_from <= ?) AND (valid_to IS NULL OR valid_to >= ?)", at, at)
	}
	var m LedgerFxRate
	err := q.Order("quoted_at desc").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *FxRateRepo) List(ctx context.Context, tenantID string) ([]*domain.FxRate, error) {
	q := dbFrom(ctx, r.db)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	var rows []LedgerFxRate
	if err := q.Order("quoted_at desc").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.FxRate, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

type ExchangeLegRepo struct {
	db      *gorm.DB
	cluster *Cluster
}

func NewExchangeLegRepo(db *gorm.DB) *ExchangeLegRepo { return &ExchangeLegRepo{db: db} }

func (r *ExchangeLegRepo) WithCluster(c *Cluster) *ExchangeLegRepo {
	r.cluster = c
	return r
}

func (r *ExchangeLegRepo) Create(ctx context.Context, leg *domain.ExchangeLeg) error {
	m := &LedgerExchangeLeg{
		ExchangeID: leg.ExchangeID,
		JournalID:  leg.JournalID,
		BizNo:      leg.BizNo,
		TenantID:   leg.TenantID,
		HolderType: string(leg.HolderType),
		HolderID:   leg.HolderID,
		FromAsset:  leg.FromAsset,
		FromAmount: leg.FromAmount,
		ToAsset:    leg.ToAsset,
		ToAmount:   leg.ToAmount,
		FeeAsset:   leg.FeeAsset,
		FeeAmount:  leg.FeeAmount,
		RateID:     leg.RateID,
		Rate:       leg.Rate,
		Status:     leg.Status,
	}
	return dbFrom(ctx, r.db).Create(m).Error
}

func (r *ExchangeLegRepo) GetByBizNo(ctx context.Context, tenantID, bizNo string) (*domain.ExchangeLeg, error) {
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var m LedgerExchangeLeg
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

func (r *ExchangeLegRepo) ListByRange(ctx context.Context, tenantID string, from, to time.Time) ([]*domain.ExchangeLeg, error) {
	var out []*domain.ExchangeLeg
	for _, db := range scanDBs(ctx, r.cluster, r.db) {
		var rows []LedgerExchangeLeg
		if err := db.Where("tenant_id = ? AND created_at >= ? AND created_at < ?", tenantID, from, to).
			Order("id asc").Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			out = append(out, rows[i].toDomain())
		}
	}
	return out, nil
}

type TenantRepo struct{ db *gorm.DB }

func NewTenantRepo(db *gorm.DB) *TenantRepo { return &TenantRepo{db: db} }

func (r *TenantRepo) Save(ctx context.Context, t *domain.Tenant) error {
	m := &LedgerTenant{TenantID: t.TenantID, Name: t.Name, Status: t.Status}
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "status", "updated_at"}),
	}).Create(m).Error
}

func (r *TenantRepo) Get(ctx context.Context, tenantID string) (*domain.Tenant, error) {
	var m LedgerTenant
	err := dbFrom(ctx, r.db).Where("tenant_id = ?", tenantID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *TenantRepo) List(ctx context.Context) ([]*domain.Tenant, error) {
	var rows []LedgerTenant
	if err := dbFrom(ctx, r.db).Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.Tenant, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

type LimitRepo struct {
	db      *gorm.DB
	cluster *Cluster
}

func NewLimitRepo(db *gorm.DB) *LimitRepo { return &LimitRepo{db: db} }

func (r *LimitRepo) WithCluster(c *Cluster) *LimitRepo {
	r.cluster = c
	return r
}

func (r *LimitRepo) AddUsage(ctx context.Context, tenantID, source, holderID, asset string, cmd domain.Command, date string, amount int64) (int64, int, error) {
	db := routeDB(ctx, r.cluster, r.db, holderID)
	m := &LedgerLimitUsage{
		TenantID:     tenantID,
		SourceSystem: source,
		HolderID:     holderID,
		AssetCode:    asset,
		Command:      string(cmd),
		BizDate:      date,
		Amount:       amount,
		Count:        1,
	}
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"}, {Name: "source_system"}, {Name: "holder_id"},
			{Name: "asset_code"}, {Name: "command"}, {Name: "biz_date"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"amount":     gorm.Expr("amount + ?", amount),
			"count":      gorm.Expr("count + 1"),
			"updated_at": time.Now().UTC(),
		}),
	}).Create(m).Error
	if err != nil {
		return 0, 0, err
	}
	var row LedgerLimitUsage
	if err := db.Where(
		"tenant_id = ? AND source_system = ? AND holder_id = ? AND asset_code = ? AND command = ? AND biz_date = ?",
		tenantID, source, holderID, asset, string(cmd), date,
	).First(&row).Error; err != nil {
		return 0, 0, err
	}
	return row.Amount, row.Count, nil
}
