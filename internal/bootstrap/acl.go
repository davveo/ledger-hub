package bootstrap

import (
	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/domain"
)

func ACL(cfg config.ACLConfig) *application.ACL {
	rules := make([]domain.ACLRule, 0, len(cfg.Rules))
	for _, r := range cfg.Rules {
		rules = append(rules, domain.ACLRule{
			SourceSystem: r.SourceSystem,
			Commands:     r.Commands,
			Assets:       r.Assets,
		})
	}
	return application.NewACL(rules)
}
