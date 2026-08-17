package application

import (
	"math/big"
	"strings"

	"github.com/davveo/ledger-hub/internal/domain"
)

func ExpectedToAmount(fromAmount int64, fromPrec, toPrec int, rate string) (int64, error) {
	if fromAmount <= 0 {
		return 0, domain.ErrInvalidParam
	}
	r := strings.TrimSpace(rate)
	if r == "" {
		return 0, domain.Keyed(domain.CodeFxRateMissing, domain.KeyFxRateMissing)
	}
	from := new(big.Rat).SetInt64(fromAmount)
	scaleFrom := new(big.Rat).SetInt64(pow10(fromPrec))
	from.Quo(from, scaleFrom)
	rv, ok := new(big.Rat).SetString(r)
	if !ok {
		return 0, domain.Keyed(domain.CodeFxRateFormat, domain.KeyFxRateFormat)
	}
	from.Mul(from, rv)
	scaleTo := new(big.Rat).SetInt64(pow10(toPrec))
	from.Mul(from, scaleTo)
	return floorRat(from), nil
}

func WithinTolerance(expected, actual, tolerance int64) bool {
	if tolerance < 0 {
		tolerance = 0
	}
	d := expected - actual
	if d < 0 {
		d = -d
	}
	return d <= tolerance
}

func pow10(n int) int64 {
	if n <= 0 {
		return 1
	}
	v := int64(1)
	for i := 0; i < n; i++ {
		v *= 10
	}
	return v
}

func floorRat(r *big.Rat) int64 {
	n := new(big.Int).Quo(r.Num(), r.Denom())
	if r.Sign() < 0 && new(big.Int).Rem(r.Num(), r.Denom()).Sign() != 0 {
		n.Add(n, big.NewInt(-1))
	}
	return n.Int64()
}
