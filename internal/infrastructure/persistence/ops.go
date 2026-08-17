package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

type AlertRepo struct{ db *gorm.DB }

func NewAlertRepo(db *gorm.DB) *AlertRepo { return &AlertRepo{db: db} }

func (r *AlertRepo) Create(ctx context.Context, a *domain.LimitAlert) error {
	if a == nil {
		return domain.ErrInvalidParam
	}
	if a.AlertID == "" {
		a.AlertID = idgen.New("alt_")
	}
	if a.At.IsZero() {
		a.At = now()
	}
	return dbFrom(ctx, r.db).Create(&LedgerLimitAlert{
		AlertID:      a.AlertID,
		TenantID:     a.TenantID,
		SourceSystem: a.SourceSystem,
		HolderID:     a.HolderID,
		AssetCode:    a.AssetCode,
		Command:      string(a.Command),
		Reason:       a.Reason,
		CreatedAt:    a.At,
	}).Error
}

func (r *AlertRepo) List(ctx context.Context, tenantID string, limit int) ([]*domain.LimitAlert, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	q := dbFrom(ctx, r.db).Order("id desc").Limit(limit)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	var rows []LedgerLimitAlert
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.LimitAlert, 0, len(rows))
	for i := range rows {
		out = append(out, &domain.LimitAlert{
			AlertID:      rows[i].AlertID,
			At:           rows[i].CreatedAt,
			TenantID:     rows[i].TenantID,
			SourceSystem: rows[i].SourceSystem,
			HolderID:     rows[i].HolderID,
			AssetCode:    rows[i].AssetCode,
			Command:      domain.Command(rows[i].Command),
			Reason:       rows[i].Reason,
		})
	}
	return out, nil
}

type OpsAuditRepo struct{ db *gorm.DB }

func NewOpsAuditRepo(db *gorm.DB) *OpsAuditRepo { return &OpsAuditRepo{db: db} }

func (r *OpsAuditRepo) Create(ctx context.Context, a *domain.OpsAudit) error {
	if a == nil || a.Action == "" {
		return domain.ErrInvalidParam
	}
	if a.AuditID == "" {
		a.AuditID = idgen.New("opa_")
	}
	if a.Operator == "" {
		a.Operator = "console"
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now()
	}
	return dbFrom(ctx, r.db).Create(&LedgerOpsAudit{
		AuditID:   a.AuditID,
		Operator:  a.Operator,
		Action:    a.Action,
		TenantID:  a.TenantID,
		Target:    a.Target,
		Detail:    a.Detail,
		CreatedAt: a.CreatedAt,
	}).Error
}

func (r *OpsAuditRepo) List(ctx context.Context, q domain.AuditQuery) ([]*domain.OpsAudit, error) {
	q = q.Clamp(50, 500)
	db := dbFrom(ctx, r.db).Order("id desc").Limit(q.Limit)
	if q.From != nil {
		db = db.Where("created_at >= ?", *q.From)
	}
	if q.To != nil {
		db = db.Where("created_at < ?", *q.To)
	}
	if q.TenantID != "" {
		db = db.Where("tenant_id = ? OR tenant_id = ''", q.TenantID)
	}
	if q.Operator != "" {
		db = db.Where("operator = ?", q.Operator)
	}
	var rows []LedgerOpsAudit
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.OpsAudit, 0, len(rows))
	for i := range rows {
		out = append(out, &domain.OpsAudit{
			AuditID:   rows[i].AuditID,
			Operator:  rows[i].Operator,
			Action:    rows[i].Action,
			TenantID:  rows[i].TenantID,
			Target:    rows[i].Target,
			Detail:    rows[i].Detail,
			CreatedAt: rows[i].CreatedAt,
		})
	}
	return out, nil
}

type OpsRunRepo struct{ db *gorm.DB }

func NewOpsRunRepo(db *gorm.DB) *OpsRunRepo { return &OpsRunRepo{db: db} }

func (r *OpsRunRepo) Save(ctx context.Context, run *domain.OpsRun) error {
	if run == nil || run.Name == "" {
		return domain.ErrInvalidParam
	}
	if run.RunID == "" {
		run.RunID = idgen.New("run_")
	}
	m := &LedgerOpsRun{
		RunID:      run.RunID,
		Name:       run.Name,
		TenantID:   run.TenantID,
		Status:     run.Status,
		Detail:     run.Detail,
		Count:      run.Count,
		InstanceID: run.InstanceID,
		Attempt:    run.Attempt,
		LastError:  run.LastError,
		DurationMs: run.DurationMs,
		StartedAt:  run.StartedAt,
		FinishedAt: run.FinishedAt,
	}
	return dbFrom(ctx, r.db).Create(m).Error
}

func (r *OpsRunRepo) List(ctx context.Context, limit int) ([]*domain.OpsRun, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var rows []LedgerOpsRun
	if err := dbFrom(ctx, r.db).Order("id desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.OpsRun, 0, len(rows))
	for i := range rows {
		out = append(out, &domain.OpsRun{
			RunID:      rows[i].RunID,
			Name:       rows[i].Name,
			TenantID:   rows[i].TenantID,
			Status:     rows[i].Status,
			Detail:     rows[i].Detail,
			Count:      rows[i].Count,
			InstanceID: rows[i].InstanceID,
			Attempt:    rows[i].Attempt,
			LastError:  rows[i].LastError,
			DurationMs: rows[i].DurationMs,
			StartedAt:  rows[i].StartedAt,
			FinishedAt: rows[i].FinishedAt,
		})
	}
	return out, nil
}

func (r *OpsRunRepo) LastSuccess(ctx context.Context) (map[string]time.Time, error) {
	type row struct {
		Name       string
		FinishedAt time.Time
	}
	var rows []row
	err := dbFrom(ctx, r.db).Model(&LedgerOpsRun{}).
		Select("name, MAX(finished_at) AS finished_at").
		Where("status = ? AND finished_at IS NOT NULL", "done").
		Group("name").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[string]time.Time{}
	for _, x := range rows {
		out[x.Name] = x.FinishedAt
	}
	return out, nil
}
