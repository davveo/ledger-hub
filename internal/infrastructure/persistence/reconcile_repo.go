package persistence

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/davveo/ledger-hub/internal/domain"
)

type ReconcileRepo struct{ db *gorm.DB }

func NewReconcileRepo(db *gorm.DB) *ReconcileRepo { return &ReconcileRepo{db: db} }

func (r *ReconcileRepo) CreateJob(ctx context.Context, job *domain.ReconcileJob) error {
	return dbFrom(ctx, r.db).Create(jobToModel(job)).Error
}

func (r *ReconcileRepo) UpdateJob(ctx context.Context, job *domain.ReconcileJob) error {
	m := jobToModel(job)
	return dbFrom(ctx, r.db).Model(&LedgerReconcileJob{}).Where("job_id = ?", job.JobID).Updates(map[string]interface{}{
		"status":       m.Status,
		"summary_json": m.SummaryJSON,
		"note":         m.Note,
	}).Error
}

func (r *ReconcileRepo) GetJob(ctx context.Context, jobID string) (*domain.ReconcileJob, error) {
	var m LedgerReconcileJob
	err := dbFrom(ctx, r.db).Where("job_id = ?", jobID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return jobFromModel(&m), nil
}

func (r *ReconcileRepo) ListJobs(ctx context.Context, tenantID string, limit int) ([]*domain.ReconcileJob, error) {
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
	var rows []LedgerReconcileJob
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.ReconcileJob, 0, len(rows))
	for i := range rows {
		out = append(out, jobFromModel(&rows[i]))
	}
	return out, nil
}

func (r *ReconcileRepo) LatestJob(ctx context.Context, tenantID, date, sourceSystem, assetCode string) (*domain.ReconcileJob, error) {
	q := dbFrom(ctx, r.db).Where("tenant_id = ? AND biz_date = ?", tenantID, date)
	if sourceSystem != "" {
		q = q.Where("source_system = ?", sourceSystem)
	}
	if assetCode != "" {
		q = q.Where("asset_code = ?", assetCode)
	}
	var m LedgerReconcileJob
	err := q.Order("id desc").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return jobFromModel(&m), nil
}

func (r *ReconcileRepo) CreateDiffs(ctx context.Context, diffs []*domain.ReconcileDiff) error {
	if len(diffs) == 0 {
		return nil
	}
	rows := make([]LedgerReconcileDiff, 0, len(diffs))
	for _, d := range diffs {
		rows = append(rows, LedgerReconcileDiff{
			DiffID:       d.DiffID,
			JobID:        d.JobID,
			Kind:         d.Kind,
			BizNo:        d.BizNo,
			Command:      string(d.Command),
			AssetCode:    d.AssetCode,
			BizAmount:    d.BizAmount,
			LedgerAmount: d.LedgerAmount,
			AccountID:    d.AccountID,
			Status:       d.Status,
			Note:         d.Note,
		})
	}
	return dbFrom(ctx, r.db).Create(&rows).Error
}

func (r *ReconcileRepo) ListDiffs(ctx context.Context, jobID string) ([]*domain.ReconcileDiff, error) {
	var rows []LedgerReconcileDiff
	if err := dbFrom(ctx, r.db).Where("job_id = ?", jobID).Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.ReconcileDiff, 0, len(rows))
	for i := range rows {
		out = append(out, diffFromModel(&rows[i]))
	}
	return out, nil
}

func (r *ReconcileRepo) ResolveDiff(ctx context.Context, diffID, note string) error {
	res := dbFrom(ctx, r.db).Model(&LedgerReconcileDiff{}).Where("diff_id = ?", diffID).Updates(map[string]interface{}{
		"status": domain.DiffStatusResolved,
		"note":   note,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func jobToModel(job *domain.ReconcileJob) *LedgerReconcileJob {
	sum := ""
	if job.Summary != nil {
		b, _ := json.Marshal(job.Summary)
		sum = string(b)
	}
	return &LedgerReconcileJob{
		JobID:        job.JobID,
		TenantID:     job.TenantID,
		BizDate:      job.Date,
		SourceSystem: job.SourceSystem,
		AssetCode:    job.AssetCode,
		Status:       job.Status,
		SummaryJSON:  sum,
		Note:         job.Note,
		CreatedAt:    job.CreatedAt,
	}
}

func jobFromModel(m *LedgerReconcileJob) *domain.ReconcileJob {
	job := &domain.ReconcileJob{
		JobID:        m.JobID,
		TenantID:     m.TenantID,
		Date:         m.BizDate,
		SourceSystem: m.SourceSystem,
		AssetCode:    m.AssetCode,
		Status:       m.Status,
		Note:         m.Note,
		CreatedAt:    m.CreatedAt,
	}
	if m.SummaryJSON != "" {
		var s domain.ReconcileSummary
		if json.Unmarshal([]byte(m.SummaryJSON), &s) == nil {
			job.Summary = &s
		}
	}
	return job
}

func diffFromModel(m *LedgerReconcileDiff) *domain.ReconcileDiff {
	return &domain.ReconcileDiff{
		DiffID:       m.DiffID,
		JobID:        m.JobID,
		Kind:         m.Kind,
		BizNo:        m.BizNo,
		Command:      domain.Command(m.Command),
		AssetCode:    m.AssetCode,
		BizAmount:    m.BizAmount,
		LedgerAmount: m.LedgerAmount,
		AccountID:    m.AccountID,
		Status:       m.Status,
		Note:         m.Note,
	}
}
