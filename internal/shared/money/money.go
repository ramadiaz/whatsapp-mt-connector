package money

import (
	"fmt"

	"github.com/shopspring/decimal"
)

func FormatRupiah(amount decimal.Decimal) string {
	f, _ := amount.Float64()
	return fmt.Sprintf("Rp%s", formatWithComma(int64(f)))
}

func formatWithComma(n int64) string {
	s := fmt.Sprintf("%d", n)
	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += "."
		}
		result += string(c)
	}
	return result
}
