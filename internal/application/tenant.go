package application

import (
	"context"

	"github.com/davveo/ledger-hub/internal/domain"
)

type TenantService struct {
	store domain.TenantRepository
}

func NewTenantService(store domain.TenantRepository) *TenantService {
	return &TenantService{store: store}
}

func (s *TenantService) Save(ctx context.Context, t *domain.Tenant) error {
	if t.TenantID == "" || t.Name == "" {
		return domain.ErrInvalidParam
	}
	if t.Status == "" {
		t.Status = "active"
	}
	return s.store.Save(ctx, t)
}

func (s *TenantService) Get(ctx context.Context, id string) (*domain.Tenant, error) {
	return s.store.Get(ctx, id)
}

func (s *TenantService) List(ctx context.Context) ([]*domain.Tenant, error) {
	return s.store.List(ctx)
}

func (s *TenantService) SetStatus(ctx context.Context, tenantID, status string) (*domain.Tenant, error) {
	if status != "active" && status != "disabled" {
		return nil, domain.ErrInvalidParam
	}
	t, err := s.store.Get(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	t.Status = status
	if err := s.store.Save(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TenantService) Ensure(ctx context.Context, tenantID string) error {
	if s == nil || s.store == nil || tenantID == "" {
		return nil
	}
	all, err := s.store.List(ctx)
	if err != nil || len(all) == 0 {
		return err
	}
	t, err := s.store.Get(ctx, tenantID)
	if err != nil {
		return domain.Keyed(domain.CodeUnknownTenant, domain.KeyUnknownTenant)
	}
	if t.Status != "active" {
		return domain.Keyed(domain.CodeTenantDisabled, domain.KeyTenantDisabled)
	}
	return nil
}
