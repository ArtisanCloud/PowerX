package archive_ops

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type datasetTargetFixture struct {
	TargetAuthority
	current Target
}

func (f *datasetTargetFixture) Resolve(_ context.Context, id string) (Target, error) {
	if id != f.current.ID {
		return Target{}, failure(404, "TARGET_NOT_FOUND", "fixture target unavailable")
	}
	return f.current, nil
}

func TestDatasetArchiveVersionedReadbackAndReplay(t *testing.T) {
	s, remote, target, _ := s3Fixture(t)
	ctx := archiveRoot()
	require.NoError(t, s.TestTarget(ctx, target.ID))
	target, err := s.target(target.ID)
	require.NoError(t, err)
	authority := &datasetTargetFixture{current: target}
	require.NoError(t, s.BindTargetAuthority(authority))
	binding := DatasetArchiveBinding{TargetID: target.ID, TargetRevision: target.Revision, TargetFingerprint: target.VerifiedFingerprint,
		DatasetUUID: uuid.NewString(), TenantUUID: uuid.NewString(), BatchUUID: uuid.NewString()}
	body := []byte("{\"test\":\"fixed record\"}\n")
	receipt, err := s.PutDatasetArchive(ctx, binding, body)
	require.NoError(t, err)
	require.NotEmpty(t, receipt.Version)
	second, err := s.PutDatasetArchive(ctx, binding, body)
	require.NoError(t, err)
	require.Equal(t, receipt, second)
	data, err := s.ReadDatasetArchive(ctx, binding, receipt)
	require.NoError(t, err)
	require.Equal(t, body, data)
	_, err = s.PutDatasetArchive(ctx, binding, []byte("{\"test\":\"other record\"}\n"))
	require.Error(t, err)
	wrongTenant := binding
	wrongTenant.TenantUUID = uuid.NewString()
	_, err = s.ReadDatasetArchive(ctx, wrongTenant, receipt)
	require.Error(t, err)
	remote.mu.Lock()
	remote.corrupt = true
	require.Len(t, remote.objects, 1, "same batch must never create another object version")
	remote.mu.Unlock()
	data, err = s.ReadDatasetArchive(ctx, binding, receipt)
	require.Error(t, err)
	require.Nil(t, data)
	authority.current.Revision++
	_, err = s.ReadDatasetArchive(ctx, binding, receipt)
	require.Error(t, err)
}
