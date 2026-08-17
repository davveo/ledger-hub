package persistence

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

type SagaRepo struct{ db *gorm.DB }

func NewSagaRepo(db *gorm.DB) *SagaRepo { return &SagaRepo{db: db} }

func (r *SagaRepo) Create(ctx context.Context, s *domain.TransferSaga) error {
	if s == nil || s.BizNo == "" {
		return domain.ErrInvalidParam
	}
	if s.SagaID == "" {
		s.SagaID = idgen.New("sg_")
	}
	now := now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	return dbFrom(ctx, r.db).Create(sagaToModel(s)).Error
}

func (r *SagaRepo) Update(ctx context.Context, s *domain.TransferSaga) error {
	if s == nil || s.SagaID == "" {
		return domain.ErrInvalidParam
	}
	s.UpdatedAt = now()
	m := sagaToModel(s)
	return dbFrom(ctx, r.db).Model(&LedgerTransferSaga{}).Where("saga_id = ?", s.SagaID).Updates(map[string]interface{}{
		"status":        m.Status,
		"out_biz_no":    m.OutBizNo,
		"in_biz_no":     m.InBizNo,
		"rollback_no":   m.RollbackNo,
		"result_json":   m.ResultJSON,
		"last_error":    m.LastError,
		"retry_count":   m.RetryCount,
		"updated_at":    m.UpdatedAt,
	}).Error
}

func (r *SagaRepo) Get(ctx context.Context, sagaID string) (*domain.TransferSaga, error) {
	var m LedgerTransferSaga
	err := dbFrom(ctx, r.db).Where("saga_id = ?", sagaID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return sagaFromModel(&m), nil
}

func (r *SagaRepo) GetByBizNo(ctx context.Context, tenantID, sourceSystem, bizNo string) (*domain.TransferSaga, error) {
	var m LedgerTransferSaga
	err := dbFrom(ctx, r.db).Where("tenant_id = ? AND source_system = ? AND biz_no = ?", tenantID, sourceSystem, bizNo).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return sagaFromModel(&m), nil
}

func (r *SagaRepo) ListOpen(ctx context.Context, tenantID string, limit int) ([]*domain.TransferSaga, error) {
	return r.List(ctx, tenantID, "", limit)
}

func (r *SagaRepo) List(ctx context.Context, tenantID, status string, limit int) ([]*domain.TransferSaga, error) {
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
	if status != "" {
		q = q.Where("status = ?", status)
	} else {
		q = q.Where("status NOT IN ?", []string{domain.SagaCompleted, domain.SagaFailed})
	}
	var rows []LedgerTransferSaga
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.TransferSaga, 0, len(rows))
	for i := range rows {
		out = append(out, sagaFromModel(&rows[i]))
	}
	return out, nil
}

func sagaToModel(s *domain.TransferSaga) *LedgerTransferSaga {
	return &LedgerTransferSaga{
		SagaID:       s.SagaID,
		TenantID:     s.TenantID,
		SourceSystem: s.SourceSystem,
		BizNo:        s.BizNo,
		FromType:     string(s.FromType),
		FromID:       s.FromID,
		ToType:       string(s.ToType),
		ToID:         s.ToID,
		AssetCode:    s.AssetCode,
		Amount:       s.Amount,
		Status:       s.Status,
		OutBizNo:     s.OutBizNo,
		InBizNo:      s.InBizNo,
		RollbackNo:   s.RollbackNo,
		ResultJSON:   s.ResultJSON,
		LastError:    s.LastError,
		RetryCount:   s.RetryCount,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
	}
}

func sagaFromModel(m *LedgerTransferSaga) *domain.TransferSaga {
	return &domain.TransferSaga{
		SagaID:       m.SagaID,
		TenantID:     m.TenantID,
		SourceSystem: m.SourceSystem,
		BizNo:        m.BizNo,
		FromType:     domain.HolderType(m.FromType),
		FromID:       m.FromID,
		ToType:       domain.HolderType(m.ToType),
		ToID:         m.ToID,
		AssetCode:    m.AssetCode,
		Amount:       m.Amount,
		Status:       m.Status,
		OutBizNo:     m.OutBizNo,
		InBizNo:      m.InBizNo,
		RollbackNo:   m.RollbackNo,
		ResultJSON:   m.ResultJSON,
		LastError:    m.LastError,
		RetryCount:   m.RetryCount,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

