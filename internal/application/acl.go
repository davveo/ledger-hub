package application

import "github.com/davveo/ledger-hub/internal/domain"

type ACL struct {
	rules []domain.ACLRule
}

func NewACL(rules []domain.ACLRule) *ACL {
	return &ACL{rules: rules}
}

func (a *ACL) Allow(source string, cmd domain.Command, asset string) bool {
	if a == nil || len(a.rules) == 0 {
		return true
	}
	for _, r := range a.rules {
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
	if a == nil || len(a.rules) == 0 {
		return nil
	}
	if !a.Allow(req.SourceSystem, req.Command, req.AssetCode) {
		return domain.ErrForbidden
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
