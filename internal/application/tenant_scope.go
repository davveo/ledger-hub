package application

import "github.com/davveo/ledger-hub/internal/domain"

func tenantMatch(got, want string) error {
	if want == "" || got != want {
		return domain.ErrNotFound
	}
	return nil
}
