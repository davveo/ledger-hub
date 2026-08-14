package application

import "github.com/davveo/ledger-hub/internal/domain"

func HolderAllowed(asset *domain.Asset, holder domain.Holder) error {
	if asset == nil {
		return domain.ErrInvalidParam
	}
	if holder.Type == domain.HolderSystemSubject {
		return nil
	}
	if len(asset.HolderTypes) == 0 {
		return nil
	}
	want := string(holder.Type)
	for _, t := range asset.HolderTypes {
		if t == want || t == "*" {
			return nil
		}
	}
	return domain.NewError(domain.CodeInvalidParam, "持有者类型不被该资产允许")
}

func AccountUsable(acc *domain.Account) error {
	if acc == nil {
		return domain.ErrNotFound
	}
	if acc.Status == domain.AccountDisabled {
		return domain.NewError(domain.CodeInvalidParam, "账户已停用")
	}
	return nil
}

func systemOverdraft(acc *domain.Account) bool {
	return acc != nil && acc.HolderType == domain.HolderSystemSubject
}
