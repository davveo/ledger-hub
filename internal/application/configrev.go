package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

func SaveConfigRevision(ctx context.Context, repo domain.ConfigRevisionRepository, operator string, acl []domain.ACLRule, limits []domain.LimitRule) (*domain.ConfigRevision, error) {
	if repo == nil {
		return nil, nil
	}
	payload, err := json.Marshal(struct {
		ACL    []domain.ACLRule   `json:"acl"`
		Limits []domain.LimitRule `json:"limits"`
	}{ACL: acl, Limits: limits})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(payload)
	rev := &domain.ConfigRevision{
		Operator:  operator,
		Checksum:  hex.EncodeToString(sum[:]),
		Payload:   string(payload),
		AppliedAt: time.Now().UTC(),
	}
	if err := repo.Create(ctx, rev); err != nil {
		return nil, err
	}
	return rev, nil
}
