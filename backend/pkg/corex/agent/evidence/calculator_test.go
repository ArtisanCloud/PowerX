package evidence

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateUsesBindingsAndDeterministicRounding(t *testing.T) {
	for _, tt := range []struct{ a, b, want string }{{"29", "34.2", "0.848"}, {"46.2", "34.2", "1.351"}, {"17", "80", "0.213"}} {
		got, err := Calculate(context.Background(), "a/b", map[string]string{"a": tt.a, "b": tt.b}, 3, false)
		require.NoError(t, err)
		require.Equal(t, tt.want, got)
	}
}

func TestCalculateRejectsMissingBindingsAndExecutableCode(t *testing.T) {
	for _, expression := range []string{"a/0", "a/missing", "exec(a)", "a.b", "a[0]", "a**2"} {
		_, err := Calculate(context.Background(), expression, map[string]string{"a": "1"}, 2, false)
		require.Error(t, err, expression)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Calculate(ctx, "a", map[string]string{"a": "1"}, 2, false)
	require.ErrorIs(t, err, context.Canceled)
}
