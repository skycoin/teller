package mathutil

import (
	"errors"
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

//Wei2Gwei convert wei to gwei 1e9wei = 1gwei
func Wei2Gwei(wei *big.Int) int64 {
	return big.NewInt(1).Div(wei, big.NewInt(1e9)).Int64()
}

//Gwei2Wei convert gwei to wei 1gwei = 1e9wei
func Gwei2Wei(gwei int64) *big.Int {
	return big.NewInt(1).Mul(big.NewInt(gwei), big.NewInt(1e9))
}

// ParseRate parses an exchange rate string and validates it
func ParseRate(rate string) (decimal.Decimal, error) {
	r, err := DecimalFromString(rate)
	if err != nil {
		return decimal.Decimal{}, err
	}

	if r.LessThanOrEqual(decimal.New(0, 0)) {
		return decimal.Decimal{}, errors.New("rate must be greater than zero")
	}

	return r, nil
}
