package indicator

import (
	"math/big"
	"strings"
)

func formatValue(value *big.Rat, decimals int) string {
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	numerator := new(big.Int).Abs(value.Num())
	numerator.Mul(numerator, factor)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, value.Denom(), remainder)
	if remainder.Lsh(remainder, 1).Cmp(value.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	display := quotient.String()
	if decimals > 0 {
		if len(display) <= decimals {
			display = strings.Repeat("0", decimals-len(display)+1) + display
		}
		point := len(display) - decimals
		display = display[:point] + "." + display[point:]
	}
	if value.Sign() < 0 && quotient.Sign() != 0 {
		display = "-" + display
	}
	return display
}
