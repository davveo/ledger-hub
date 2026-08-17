package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/davveo/ledger-hub/internal/domain"
)

type InboxRepo struct{ db *gorm.DB }

func NewInboxRepo(db *gorm.DB) *InboxRepo { return &InboxRepo{db: db} }

func (r *InboxRepo) InsertIfAbsent(ctx context.Context, m *domain.InboxMessage) (bool, error) {
	if m == nil || m.MessageID == "" {
		return false, domain.ErrInvalidParam
	}
	if m.Status == "" {
		m.Status = domain.InboxPending
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = 1
	}
	now := now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	res := dbFrom(ctx, r.db).Where("message_id = ?", m.MessageID).Attrs(LedgerMQInbox{
		MessageID:     m.MessageID,
		Topic:         m.Topic,
		SchemaVersion: m.SchemaVersion,
		Payload:       m.Payload,
		Status:        m.Status,
		Attempts:      m.Attempts,
		LastError:     m.LastError,
		NextRetryAt:   m.NextRetryAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}).FirstOrCreate(&LedgerMQInbox{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *InboxRepo) Get(ctx context.Context, messageID string) (*domain.InboxMessage, error) {
	var m LedgerMQInbox
	err := dbFrom(ctx, r.db).Where("message_id = ?", messageID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return inboxFromModel(&m), nil
}

func (r *InboxRepo) Update(ctx context.Context, m *domain.InboxMessage) error {
	if m == nil || m.MessageID == "" {
		return domain.ErrInvalidParam
	}
	m.UpdatedAt = now()
	res := dbFrom(ctx, r.db).Model(&LedgerMQInbox{}).Where("message_id = ?", m.MessageID).Updates(map[string]interface{}{
		"status":         m.Status,
		"attempts":       m.Attempts,
		"last_error":     m.LastError,
		"next_retry_at":  m.NextRetryAt,
		"schema_version": m.SchemaVersion,
		"updated_at":     m.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *InboxRepo) List(ctx context.Context, status string, limit int) ([]*domain.InboxMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	q := dbFrom(ctx, r.db).Order("id desc").Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var rows []LedgerMQInbox
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.InboxMessage, 0, len(rows))
	for i := range rows {
		out = append(out, inboxFromModel(&rows[i]))
	}
	return out, nil
}

func (r *InboxRepo) ListDue(ctx context.Context, t time.Time, limit int) ([]*domain.InboxMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	q := dbFrom(ctx, r.db).Where("status IN ?", []string{domain.InboxPending, domain.InboxRetry}).
		Where("next_retry_at IS NULL OR next_retry_at <= ?", t).
		Order("id asc").Limit(limit)
	var rows []LedgerMQInbox
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.InboxMessage, 0, len(rows))
	for i := range rows {
		out = append(out, inboxFromModel(&rows[i]))
	}
	return out, nil
}

func inboxFromModel(m *LedgerMQInbox) *domain.InboxMessage {
	return &domain.InboxMessage{
		MessageID:     m.MessageID,
		Topic:         m.Topic,
		SchemaVersion: m.SchemaVersion,
		Payload:       m.Payload,
		Status:        m.Status,
		Attempts:      m.Attempts,
		LastError:     m.LastError,
		NextRetryAt:   m.NextRetryAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}
