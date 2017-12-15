package mathutil

import (
	"math/big"

	"github.com/shopspring/decimal"
)

// DecimalFromString parses a string into a decimal.Decimal.
// It supports int, float and rational fraction strings.
func DecimalFromString(s string) (decimal.Decimal, error) {
	// shopspring.Decimal does not parse rational fraction strings
	// Use math/big.Rat to parse these

	// Try to parse the string with decimal first, if it succeeds, use that value
	d, err := decimal.NewFromString(s)
	if err == nil {
		return d, nil
	}

	// Try to parse the string as a rational fraction string, then convert to decimal
	r := &big.Rat{}
	_, ok := r.SetString(s)
	if !ok {
		// Return the original decimal.NewFromString error if the string is invalid,
		// since SetString doesn't return an error message
		return decimal.Decimal{}, err
	}

	// Convert the rational number to a fixed-precision float string
	t := r.FloatString(8)

	// Parse the fixed-precision float string (decimal.New doesn't support big.Rat)
	return decimal.NewFromString(t)
}
