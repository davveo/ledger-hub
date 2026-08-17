package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
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
		"phase":        m.Phase,
		"summary_json": m.SummaryJSON,
		"note":         m.Note,
		"payload_json": m.PayloadJSON,
		"version":      m.Version,
		"job_type":     m.JobType,
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

func (r *ReconcileRepo) GetJobInTenant(ctx context.Context, tenantID, jobID string) (*domain.ReconcileJob, error) {
	if tenantID == "" || jobID == "" {
		return nil, domain.ErrInvalidParam
	}
	var m LedgerReconcileJob
	err := dbFrom(ctx, r.db).Where("job_id = ? AND tenant_id = ?", jobID, tenantID).First(&m).Error
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

func (r *ReconcileRepo) ListQueuedJobs(ctx context.Context, limit int) ([]*domain.ReconcileJob, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var rows []LedgerReconcileJob
	if err := dbFrom(ctx, r.db).Where("status = ?", domain.ReconJobQueued).Order("id asc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.ReconcileJob, 0, len(rows))
	for i := range rows {
		out = append(out, jobFromModel(&rows[i]))
	}
	return out, nil
}

func (r *ReconcileRepo) FindJobByKey(ctx context.Context, tenantID, date, sourceSystem, assetCode, jobType string) (*domain.ReconcileJob, error) {
	if jobType == "" {
		jobType = domain.ReconJobTypeDaily
	}
	var m LedgerReconcileJob
	err := dbFrom(ctx, r.db).Where("tenant_id = ? AND biz_date = ? AND source_system = ? AND asset_code = ? AND job_type = ?",
		tenantID, date, sourceSystem, assetCode, jobType).Order("version desc").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return jobFromModel(&m), nil
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

func (r *ReconcileRepo) ListOpenDiffs(ctx context.Context, tenantID string, limit int) ([]*domain.ReconcileDiff, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	q := dbFrom(ctx, r.db).Table("ledger_reconcile_diff AS d").
		Select("d.*").
		Joins("JOIN ledger_reconcile_job AS j ON j.job_id = d.job_id").
		Where("d.status = ?", domain.DiffStatusOpen)
	if tenantID != "" {
		q = q.Where("j.tenant_id = ?", tenantID)
	}
	var rows []LedgerReconcileDiff
	if err := q.Order("d.id desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.ReconcileDiff, 0, len(rows))
	for i := range rows {
		out = append(out, diffFromModel(&rows[i]))
	}
	return out, nil
}

func (r *ReconcileRepo) GetDiff(ctx context.Context, diffID string) (*domain.ReconcileDiff, error) {
	var m LedgerReconcileDiff
	err := dbFrom(ctx, r.db).Where("diff_id = ?", diffID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return diffFromModel(&m), nil
}

func (r *ReconcileRepo) UpdateDiff(ctx context.Context, d *domain.ReconcileDiff) error {
	if d == nil || d.DiffID == "" {
		return domain.ErrInvalidParam
	}
	res := dbFrom(ctx, r.db).Model(&LedgerReconcileDiff{}).Where("diff_id = ?", d.DiffID).Updates(map[string]interface{}{
		"status":      d.Status,
		"note":        d.Note,
		"assignee":    d.Assignee,
		"resolved_by": d.ResolvedBy,
		"closed_by":   d.ClosedBy,
		"closed_at":   d.ClosedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ReconcileRepo) ResolveDiff(ctx context.Context, diffID, note, operator string) error {
	now := time.Now().UTC()
	res := dbFrom(ctx, r.db).Model(&LedgerReconcileDiff{}).Where("diff_id = ?", diffID).Updates(map[string]interface{}{
		"status":      domain.DiffStatusResolved,
		"note":        note,
		"resolved_by": operator,
		"closed_by":   operator,
		"closed_at":   now,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ReconcileRepo) CreateDiffEvent(ctx context.Context, ev *domain.ReconcileDiffEvent) error {
	if ev == nil || ev.DiffID == "" || ev.Action == "" {
		return domain.ErrInvalidParam
	}
	if ev.EventID == "" {
		ev.EventID = idgen.New("rde_")
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}
	return dbFrom(ctx, r.db).Create(&LedgerReconcileDiffEvent{
		EventID:   ev.EventID,
		DiffID:    ev.DiffID,
		Action:    ev.Action,
		Operator:  ev.Operator,
		Detail:    ev.Detail,
		CreatedAt: ev.CreatedAt,
	}).Error
}

func (r *ReconcileRepo) ListDiffEvents(ctx context.Context, diffID string) ([]*domain.ReconcileDiffEvent, error) {
	var rows []LedgerReconcileDiffEvent
	if err := dbFrom(ctx, r.db).Where("diff_id = ?", diffID).Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.ReconcileDiffEvent, 0, len(rows))
	for i := range rows {
		out = append(out, &domain.ReconcileDiffEvent{
			EventID:   rows[i].EventID,
			DiffID:    rows[i].DiffID,
			Action:    rows[i].Action,
			Operator:  rows[i].Operator,
			Detail:    rows[i].Detail,
			CreatedAt: rows[i].CreatedAt,
		})
	}
	return out, nil
}

func jobToModel(job *domain.ReconcileJob) *LedgerReconcileJob {
	sum := ""
	if job.Summary != nil {
		b, _ := json.Marshal(job.Summary)
		sum = string(b)
	}
	jt := job.JobType
	if jt == "" {
		jt = domain.ReconJobTypeDaily
	}
	ver := job.Version
	if ver <= 0 {
		ver = 1
	}
	return &LedgerReconcileJob{
		JobID:        job.JobID,
		TenantID:     job.TenantID,
		BizDate:      job.Date,
		SourceSystem: job.SourceSystem,
		AssetCode:    job.AssetCode,
		JobType:      jt,
		Version:      ver,
		Status:       job.Status,
		Phase:        job.Phase,
		SummaryJSON:  sum,
		Note:         job.Note,
		PayloadJSON:  job.PayloadJSON,
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
		JobType:      m.JobType,
		Version:      m.Version,
		Status:       m.Status,
		Phase:        m.Phase,
		Note:         m.Note,
		PayloadJSON:  m.PayloadJSON,
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
		Assignee:     m.Assignee,
		ResolvedBy:   m.ResolvedBy,
		ClosedBy:     m.ClosedBy,
		ClosedAt:     m.ClosedAt,
	}
}
