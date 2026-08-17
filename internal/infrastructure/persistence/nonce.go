package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/davveo/ledger-hub/internal/domain"
)

type NonceRepo struct{ db *gorm.DB }

func NewNonceRepo(db *gorm.DB) *NonceRepo { return &NonceRepo{db: db} }

func (r *NonceRepo) Consume(ctx context.Context, clientID, nonce string, ttl time.Duration) error {
	if clientID == "" || nonce == "" {
		return domain.ErrInvalidParam
	}
	err := dbFrom(ctx, r.db).Create(&LedgerGatewayNonce{
		ClientID:  clientID,
		Nonce:     nonce,
		CreatedAt: now(),
	}).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || isDup(err) {
			return domain.ErrReplay
		}
		return err
	}
	if ttl > 0 {
		_, _ = r.DeleteBefore(ctx, time.Now().UTC().Add(-ttl))
	}
	return nil
}

func (r *NonceRepo) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	res := dbFrom(ctx, r.db).Where("created_at < ?", before).Delete(&LedgerGatewayNonce{})
	return res.RowsAffected, res.Error
}

func isDup(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "Duplicate") || strings.Contains(s, "UNIQUE") || strings.Contains(s, "unique")
}
