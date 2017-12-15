package logger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContext(t *testing.T) {
	log, err := NewLogger("", true)
	require.NoError(t, err)

	ctx := context.Background()
	require.Nil(t, FromContext(ctx))

	ctx = WithContext(ctx, log)
	require.NotNil(t, FromContext(ctx))
}
