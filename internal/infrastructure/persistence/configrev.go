package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"gorm.io/gorm"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

type ConfigRevisionRepo struct{ db *gorm.DB }

func NewConfigRevisionRepo(db *gorm.DB) *ConfigRevisionRepo { return &ConfigRevisionRepo{db: db} }

func (r *ConfigRevisionRepo) Create(ctx context.Context, rev *domain.ConfigRevision) error {
	if rev == nil {
		return domain.ErrInvalidParam
	}
	if rev.RevisionID == "" {
		rev.RevisionID = idgen.New("cr_")
	}
	if rev.AppliedAt.IsZero() {
		rev.AppliedAt = now()
	}
	if rev.Checksum == "" && rev.Payload != "" {
		sum := sha256.Sum256([]byte(rev.Payload))
		rev.Checksum = hex.EncodeToString(sum[:])
	}
	if rev.Version == 0 {
		var last LedgerConfigRevision
		err := dbFrom(ctx, r.db).Order("version desc").First(&last).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			rev.Version = last.Version + 1
		} else {
			rev.Version = 1
		}
	}
	return dbFrom(ctx, r.db).Create(&LedgerConfigRevision{
		RevisionID: rev.RevisionID,
		Version:    rev.Version,
		Operator:   rev.Operator,
		Checksum:   rev.Checksum,
		Payload:    rev.Payload,
		AppliedAt:  rev.AppliedAt,
	}).Error
}

func (r *ConfigRevisionRepo) List(ctx context.Context, limit int) ([]*domain.ConfigRevision, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var rows []LedgerConfigRevision
	if err := dbFrom(ctx, r.db).Order("version desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.ConfigRevision, 0, len(rows))
	for i := range rows {
		out = append(out, revFromModel(&rows[i]))
	}
	return out, nil
}

func (r *ConfigRevisionRepo) Latest(ctx context.Context) (*domain.ConfigRevision, error) {
	var m LedgerConfigRevision
	err := dbFrom(ctx, r.db).Order("version desc").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return revFromModel(&m), nil
}

func revFromModel(m *LedgerConfigRevision) *domain.ConfigRevision {
	return &domain.ConfigRevision{
		RevisionID: m.RevisionID,
		Version:    m.Version,
		Operator:   m.Operator,
		Checksum:   m.Checksum,
		Payload:    m.Payload,
		AppliedAt:  m.AppliedAt,
	}
}
