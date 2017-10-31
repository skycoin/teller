package mathutil

import (
	"errors"
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
