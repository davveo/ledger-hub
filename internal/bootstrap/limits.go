package bootstrap

import (
	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/domain"
)

func Limits(cfg config.LimitConfig) []domain.LimitRule {
	out := make([]domain.LimitRule, 0, len(cfg.Rules))
	for _, r := range cfg.Rules {
		out = append(out, domain.LimitRule{
			SourceSystem: r.SourceSystem,
			AssetCode:    r.AssetCode,
			Command:      domain.Command(r.Command),
			MaxAmount:    r.MaxAmount,
			DailyAmount:  r.DailyAmount,
			DailyCount:   r.DailyCount,
		})
	}
	return out
}
