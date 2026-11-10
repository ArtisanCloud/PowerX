package storagegov

import (
	"context"
	"testing"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
)

type tableFixtureCollector struct{ calls int }

func (f *tableFixtureCollector) CollectDatabase(context.Context) DatabaseUsage {
	return DatabaseUsage{}
}
func (f *tableFixtureCollector) CollectSystemTables(_ context.Context, id string) ([]repo.SystemTableObservation, error) {
	f.calls++
	return []repo.SystemTableObservation{{SystemTableDefinition: m.SystemTableCatalog()[0], State: "not_present", TimeState: "unavailable"}}, nil
}
func TestDatabaseTableInventoryIsRootOnlyCachedAndDoesNotAdmitArbitraryTables(t *testing.T) {
	clock := time.Now()
	collector := &tableFixtureCollector{}
	s, err := NewService(Options{FilesystemPath: "/", Collector: &fixtureCollector{}, DatabaseCollector: collector, Now: func() time.Time { return clock }})
	require.NoError(t, err)
	_, err = s.DatabaseTables(context.Background(), "")
	require.ErrorIs(t, err, ErrRootRequired)
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true}), true)
	_, err = s.DatabaseTables(ctx, "iam_user")
	require.ErrorIs(t, err, ErrSystemTableNotFound)
	require.Zero(t, collector.calls)
	first, err := s.DatabaseTables(ctx, "")
	require.NoError(t, err)
	require.Nil(t, first.Items[0].TotalBytes)
	require.Nil(t, first.InvocationPolicy)
	_, err = s.DatabaseTables(ctx, "")
	require.NoError(t, err)
	require.Equal(t, 1, collector.calls)
	clock = clock.Add(3 * time.Minute)
	_, err = s.DatabaseTables(ctx, "")
	require.NoError(t, err)
	require.Equal(t, 2, collector.calls)
	_, err = s.DatabaseTables(ctx, m.SystemTableCatalog()[0].Name)
	require.NoError(t, err)
	require.Equal(t, 3, collector.calls)
}

func TestDatabaseTableInventoryKeepsCatalogWhenMetricsAreUnconfigured(t *testing.T) {
	s, err := NewService(Options{FilesystemPath: "/", Collector: &fixtureCollector{}})
	require.NoError(t, err)
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true}), true)
	out, err := s.DatabaseTables(ctx, "")
	require.NoError(t, err)
	require.Len(t, out.Items, len(m.SystemTableCatalog()))
	for _, item := range out.Items {
		require.Nil(t, item.TotalBytes)
		require.Nil(t, item.OldestAt)
		require.Equal(t, "unavailable", item.State)
	}
	for _, id := range []string{"iam_authorization_tracking", "iam_authorization_streams", "iam_authorization_segments", m.TableControlConfiguration, m.TableControlRevision} {
		found := false
		for _, item := range out.Items {
			if item.Name == id {
				found = true
				require.Equal(t, "protected", item.CleanupMechanism)
			}
		}
		require.True(t, found)
	}
}
