package application

import (
	"sync"

	"github.com/davveo/ledger-hub/internal/domain"
)

type ACL struct {
	mu    sync.RWMutex
	rules []domain.ACLRule
}

func NewACL(rules []domain.ACLRule) *ACL {
	return &ACL{rules: append([]domain.ACLRule(nil), rules...)}
}

func (a *ACL) Replace(rules []domain.ACLRule) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.rules = append([]domain.ACLRule(nil), rules...)
	a.mu.Unlock()
}

func (a *ACL) Rules() []domain.ACLRule {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]domain.ACLRule(nil), a.rules...)
}

func (a *ACL) Allow(source string, cmd domain.Command, asset string) bool {
	return a.AllowTenant("", source, cmd, asset)
}

func (a *ACL) AllowTenant(tenant, source string, cmd domain.Command, asset string) bool {
	if a == nil {
		return true
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if len(a.rules) == 0 {
		return true
	}
	for _, r := range a.rules {
		if r.TenantID != "" && r.TenantID != tenant {
			continue
		}
		if r.SourceSystem != "*" && r.SourceSystem != source {
			continue
		}
		if !matchAny(r.Commands, string(cmd)) {
			continue
		}
		if asset == "" || matchAny(r.Assets, asset) {
			return true
		}
	}
	return false
}

func (a *ACL) Check(req domain.CommandRequest) error {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	empty := len(a.rules) == 0
	a.mu.RUnlock()
	if empty {
		return nil
	}
	if !a.AllowTenant(req.TenantID, req.SourceSystem, req.Command, req.AssetCode) {
		return domain.ErrForbidden
	}
	if req.Command == domain.CmdExchange && req.ToAssetCode != "" {
		if !a.AllowTenant(req.TenantID, req.SourceSystem, req.Command, req.ToAssetCode) {
			return domain.ErrForbidden
		}
	}
	return nil
}

func matchAny(list []string, v string) bool {
	if len(list) == 0 {
		return true
	}
	for _, item := range list {
		if item == "*" || item == v {
			return true
		}
	}
	return false
}
