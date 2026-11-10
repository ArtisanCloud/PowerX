package archive_ops

import (
	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryEpochQuarantinesCredentialAndTargetProof(t *testing.T) {
	s, _, target, _ := s3Fixture(t)
	ctx := credentialRootContext()
	attachOwnedCredentialVault(t, s)
	dir := filepath.Join(t.TempDir(), "maintenance")
	require.NoError(t, os.Mkdir(dir, 0700))
	anchor, e := storagerecovery.Initialize(dir, uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, s.BindRecoveryAnchor(anchor))
	values := s.cfg.S3Credentials[target.CredentialRef]
	overview, e := s.CredentialOverview(ctx)
	require.NoError(t, e)
	var row CredentialView
	for _, r := range overview.Items {
		if r.Ref == target.CredentialRef {
			row = r
		}
	}
	oldInput := CredentialControlInput{RequestID: uuid.NewString(), ExpectedRevision: row.Revision, LegacyDigest: row.LegacyDigest, Confirm: true}
	saved, e := s.AdoptS3Credential(ctx, row.Ref, oldInput)
	require.NoError(t, e)
	require.NoError(t, s.TestTarget(ctx, target.ID))
	target, e = s.target(target.ID)
	require.NoError(t, e)
	require.True(t, s.targetVerified(target))
	epoch, e := anchor.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	_, e = s.s3Credential(row.Ref)
	require.Error(t, e)
	require.False(t, s.targetVerified(target))
	replay, e := s.AdoptS3Credential(ctx, row.Ref, oldInput)
	require.NoError(t, e)
	require.Empty(t, replay.Receipt.RecoveryEpoch, "old receipt remains historical")
	_, e = s.s3Credential(row.Ref)
	require.Error(t, e, "replaying an old receipt cannot approve a new epoch")
	overview, e = s.CredentialOverview(ctx)
	require.NoError(t, e)
	for _, r := range overview.Items {
		if r.Ref == row.Ref {
			require.Equal(t, "recovery_required", r.State)
		}
	}
	fresh, e := s.SaveS3Credential(ctx, SaveS3CredentialInput{RequestID: uuid.NewString(), ExpectedRevision: saved.Receipt.Revision, Ref: row.Ref, AccessKey: values.AccessKey, SecretKey: values.SecretKey, SessionToken: values.SessionToken})
	require.NoError(t, e)
	require.Equal(t, epoch.Epoch, fresh.Receipt.RecoveryEpoch)
	_, e = s.s3Credential(row.Ref)
	require.NoError(t, e)
	require.False(t, s.targetVerified(target), "fresh credential does not refresh an old target proof")
	require.NoError(t, s.TestTarget(ctx, target.ID))
	target, e = s.target(target.ID)
	require.NoError(t, e)
	require.True(t, s.targetVerified(target))
	_, e = anchor.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	_, e = s.s3Credential(row.Ref)
	require.Error(t, e)
	require.False(t, s.targetVerified(target))
	require.NoError(t, os.Remove(filepath.Join(dir, "storage-recovery-epoch.json")))
	_, e = s.s3Credential(row.Ref)
	require.Error(t, e)
}

func TestRecoveryLegacyCredentialRequiresFreshSealedRecord(t *testing.T) {
	s, _, target, _ := s3Fixture(t)
	ctx := credentialRootContext()
	attachOwnedCredentialVault(t, s)
	old := s.cfg.S3Credentials[target.CredentialRef]
	old.Revision = 7
	s.cfg.S3Credentials[target.CredentialRef] = old
	dir := filepath.Join(t.TempDir(), "maintenance")
	require.NoError(t, os.Mkdir(dir, 0700))
	anchor, e := storagerecovery.Initialize(dir, uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, s.BindRecoveryAnchor(anchor))
	epoch, e := anchor.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	overview, e := s.CredentialOverview(ctx)
	require.NoError(t, e)
	require.Len(t, overview.Items, 1)
	row := overview.Items[0]
	require.Equal(t, "recovery_required", row.State)
	require.EqualValues(t, 7, row.Revision)
	_, e = s.AdoptS3Credential(ctx, row.Ref, CredentialControlInput{RequestID: uuid.NewString(), ExpectedRevision: row.Revision, LegacyDigest: row.LegacyDigest, Confirm: true})
	require.Error(t, e)
	out, e := s.SaveS3Credential(ctx, SaveS3CredentialInput{RequestID: uuid.NewString(), ExpectedRevision: 0, Ref: row.Ref, AccessKey: old.AccessKey, SecretKey: old.SecretKey})
	require.NoError(t, e)
	require.EqualValues(t, 1, out.Receipt.Revision)
	require.Equal(t, epoch.Epoch, out.Receipt.RecoveryEpoch)
}
