package monitorlogs

import (
	"context"
	logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLegacyArbitraryTableCleanupIsDisabledEvenWithoutLifecycleOwner(t *testing.T) {
	p := NewDBRetentionProvider(nil, 100, 100, []logcfg.RetentionDBTable{{Name: "iam_user", TimeColumn: "created_at", RetentionDays: 1}})
	n, errs := p.Cleanup(context.Background(), 1)
	require.Zero(t, n)
	require.Contains(t, errs, "legacy_database_retention_disabled_use_typed_storage_policies")
	n, per, errs := p.PreviewBreakdown(context.Background(), 1)
	require.Zero(t, n)
	require.Empty(t, per)
	require.NotEmpty(t, errs)
}
