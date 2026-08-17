package application

import (
	"fmt"
	"strconv"
)

func FormatAmount(amount int64, precision int) string {
	if precision <= 0 {
		return strconv.FormatInt(amount, 10)
	}
	neg := amount < 0
	if neg {
		amount = -amount
	}
	scale := int64(1)
	for i := 0; i < precision; i++ {
		scale *= 10
	}
	s := fmt.Sprintf("%d.%0*d", amount/scale, precision, amount%scale)
	if neg {
		return "-" + s
	}
	return s
}
