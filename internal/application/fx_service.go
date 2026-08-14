package application

import (
	"context"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

type FxService struct {
	fx domain.FxRateRepository
}

func NewFxService(fx domain.FxRateRepository) *FxService {
	return &FxService{fx: fx}
}

func (s *FxService) Save(ctx context.Context, r *domain.FxRate) error {
	if r.TenantID == "" || r.BaseAsset == "" || r.QuoteAsset == "" || r.Rate == "" {
		return domain.ErrInvalidParam
	}
	if r.RateID == "" {
		r.RateID = idgen.New("fxr_")
	}
	if r.RateSource == "" {
		r.RateSource = "manual"
	}
	if r.QuotedAt.IsZero() {
		r.QuotedAt = time.Now().UTC()
	}
	return s.fx.Save(ctx, r)
}

func (s *FxService) Quote(ctx context.Context, tenantID, base, quote string, at time.Time) (*domain.FxRate, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return s.fx.Find(ctx, tenantID, base, quote, at)
}

func (s *FxService) Get(ctx context.Context, rateID string) (*domain.FxRate, error) {
	return s.fx.Get(ctx, rateID)
}

func (s *FxService) List(ctx context.Context, tenantID string) ([]*domain.FxRate, error) {
	return s.fx.List(ctx, tenantID)
}
