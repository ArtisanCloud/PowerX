package storagegov

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type resourceHistoryCollector struct{ snap Snapshot }

func (c *resourceHistoryCollector) Collect(context.Context, string, []Resource) Snapshot {
	return c.snap
}
func TestStorageHistoryPersistsEveryRegisteredResourceAndExplainsUnknown(t *testing.T) {
	s, store, ctx := storageStateFixture(t)
	now := time.Now().UTC().Truncate(SampleInterval)
	r := Resource{ID: "app", Name: "Logs", Kind: "application_logs", Scope: "dev", InstanceID: "installed", Path: "/registered/logs"}
	s.opts.Resources = []Resource{r}
	size := int64(100)
	c := &resourceHistoryCollector{snap: Snapshot{Resources: []Usage{{Resource: r, FilesystemID: "volume", State: "observed", AttributedBytes: &size, Files: 3}}}}
	s.opts.Collector = c
	s.opts.Now = func() time.Time { return now }
	require.NoError(t, s.CaptureSample(ctx))
	size = 160
	now = now.Add(SampleInterval)
	require.NoError(t, s.CaptureSample(ctx))
	now = now.Add(SampleInterval)
	out, err := s.History(ctx)
	require.NoError(t, err)
	require.Len(t, out.Sources, 1)
	require.Len(t, out.Points, 2)
	require.EqualValues(t, 100, *out.Points[0].Resources[0].Bytes)
	require.Nil(t, out.Points[0].Resources[0].NetChangeBytes)
	require.EqualValues(t, 60, *out.Points[1].Resources[0].NetChangeBytes)
	require.Equal(t, "not_recorded", out.Points[0].DatabaseProblem)
	// 统计路径/实例归属发生变化时，不跨口径相减。
	c.snap.Resources[0].Path = "/another/source"
	size = 12
	require.NoError(t, s.CaptureSample(ctx))
	now = now.Add(SampleInterval)
	out, err = s.History(ctx)
	require.NoError(t, err)
	require.Nil(t, out.Points[2].Resources[0].NetChangeBytes)
	require.LessOrEqual(t, len(store.samples[0].Document), 32768)
}
