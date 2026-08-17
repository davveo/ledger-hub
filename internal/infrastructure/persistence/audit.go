package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

type AuditRepo struct{ db *gorm.DB }

func NewAuditRepo(db *gorm.DB) *AuditRepo { return &AuditRepo{db: db} }

func (r *AuditRepo) Create(ctx context.Context, a *domain.GatewayAudit) error {
	if a == nil {
		return domain.ErrInvalidParam
	}
	if a.AuditID == "" {
		a.AuditID = idgen.New("aud_")
	}
	return dbFrom(ctx, r.db).Create(&LedgerGatewayAudit{
		AuditID:    a.AuditID,
		ClientID:   a.ClientID,
		TenantID:   a.TenantID,
		Method:     a.Method,
		Path:       a.Path,
		Status:     a.Status,
		RemoteAddr: a.RemoteAddr,
		RequestID:  a.RequestID,
		Operator:   a.Operator,
		CreatedAt:  a.CreatedAt,
	}).Error
}

func (r *AuditRepo) List(ctx context.Context, q domain.AuditQuery) ([]*domain.GatewayAudit, error) {
	q = q.Clamp(50, 500)
	db := dbFrom(ctx, r.db).Order("id desc").Limit(q.Limit)
	db = applyAuditTime(db, q)
	if q.TenantID != "" {
		db = db.Where("tenant_id = ?", q.TenantID)
	}
	if q.ClientID != "" {
		db = db.Where("client_id = ?", q.ClientID)
	}
	if q.Operator != "" {
		db = db.Where("operator = ?", q.Operator)
	}
	var rows []LedgerGatewayAudit
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.GatewayAudit, 0, len(rows))
	for i := range rows {
		out = append(out, &domain.GatewayAudit{
			AuditID:    rows[i].AuditID,
			ClientID:   rows[i].ClientID,
			TenantID:   rows[i].TenantID,
			Method:     rows[i].Method,
			Path:       rows[i].Path,
			Status:     rows[i].Status,
			RemoteAddr: rows[i].RemoteAddr,
			RequestID:  rows[i].RequestID,
			Operator:   rows[i].Operator,
			CreatedAt:  rows[i].CreatedAt,
		})
	}
	return out, nil
}

func applyAuditTime(db *gorm.DB, q domain.AuditQuery) *gorm.DB {
	if q.From != nil {
		db = db.Where("created_at >= ?", *q.From)
	}
	if q.To != nil {
		db = db.Where("created_at < ?", *q.To)
	}
	return db
}
