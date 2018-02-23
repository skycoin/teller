package mathutil

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestDecimalFromString(t *testing.T) {
	cases := []struct {
		s      string
		result decimal.Decimal
		err    error
	}{
		{
			s:   "bad",
			err: errors.New("can't convert bad to decimal"),
		},

		{
			s:   "1/0",
			err: errors.New("can't convert 1/0 to decimal"),
		},

		{
			s:      "-1",
			result: decimal.New(-1, 0),
		},

		{
			s:      "0.1",
			result: decimal.New(1, -1),
		},

		{
			s:      "1/10",
			result: decimal.New(1, -1),
		},
	}

	for _, tc := range cases {
		t.Run(tc.s, func(t *testing.T) {
			d, err := DecimalFromString(tc.s)
			require.True(t, tc.result.Equal(d))
			require.Equal(t, tc.err, err)
		})
	}
}

func TestWei2Gwei(t *testing.T) {
	cases := []struct {
		wei  *big.Int
		gwei int64
	}{
		{
			wei:  big.NewInt(0),
			gwei: 0,
		},
		{
			wei:  big.NewInt(1e18),
			gwei: 1e9,
		},
		{
			wei:  big.NewInt(1).Mul(big.NewInt(1e18), big.NewInt(1e3)),
			gwei: 1e12,
		},
		{
			wei:  big.NewInt(1).Mul(big.NewInt(1e18), big.NewInt(1e6)),
			gwei: 1e15,
		},
	}
	for _, tc := range cases {
		name := fmt.Sprintf("wei=%v gwei=%d", tc.wei, tc.gwei)
		t.Run(name, func(t *testing.T) {
			result := Wei2Gwei(tc.wei)
			require.Equal(t, tc.gwei, result, "%d == %d", tc.gwei, result)
		})
	}
	for _, tc := range cases {
		name := fmt.Sprintf("wei=%v gwei=%d", tc.wei, tc.gwei)
		t.Run(name, func(t *testing.T) {
			result := Gwei2Wei(tc.gwei)
			require.Equal(t, 0, tc.wei.Cmp(result), "%v == %v", tc.wei, result)
		})
	}
}
