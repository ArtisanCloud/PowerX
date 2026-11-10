package backup_ops

import (
	"context"
	"errors"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupReviewVersionBindsIndependentRecoveryGeneration(t *testing.T) {
	row := repo.CleanupReviewRow{ID: 1, JobID: 2, PolicyID: 3, JobFound: true, CleanupState: "unconfirmed", CleanupToken: uuid.NewString()}
	before := boundCleanupReviewVersion(row, strings.Repeat("a", 64))
	require.NotEqual(t, cleanupReviewVersion(row), before, "metadata-only legacy versions must not authorize v3 retention")
	require.NotEqual(t, before, boundCleanupReviewVersion(row, strings.Repeat("b", 64)), "identical restored metadata belongs to a different generation")
	require.Equal(t, before, boundCleanupReviewVersion(row, strings.Repeat("a", 64)))
	row.JobProtected = true
	require.NotEqual(t, before, boundCleanupReviewVersion(row, strings.Repeat("a", 64)))
}

func TestCleanupReviewMetadataNeverProvesDeletionOrIntegrity(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", root)
	a := testArtifact(t, root, "one.dump")
	path := strings.TrimPrefix(a.StorageURI, "file://")
	row := repo.CleanupReviewRow{ID: 1, JobID: 1, JobFound: true, CleanupState: "unconfirmed", CleanupToken: uuid.NewString(), StorageURI: a.StorageURI, SizeBytes: a.SizeBytes, Checksum: a.Checksum}
	for _, suffix := range []string{".json", ".sha256"} {
		require.NoError(t, os.WriteFile(path+suffix, []byte("unverified sidecar"), 0600))
	}
	inspect := func(state string) {
		t.Helper()
		out, e := inspectCleanupFiles(context.Background(), row)
		require.NoError(t, e)
		require.Equal(t, state, out.State)
		require.True(t, out.ReadOnly)
		require.False(t, out.ContentVerified)
		require.Equal(t, row.CleanupToken, out.Item.Token)
	}
	inspect("present_unverified")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", int(row.SizeBytes))), 0600))
	inspect("present_unverified")
	require.NoError(t, os.WriteFile(path, []byte("short"), 0600))
	inspect("size_changed")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", int(row.SizeBytes))), 0600))
	require.NoError(t, os.Remove(path+".json"))
	inspect("partial")
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Remove(path+".sha256"))
	inspect("missing")
	require.Equal(t, "unconfirmed", row.CleanupState)
	require.NotEmpty(t, row.CleanupToken)
	row.SizeBytes = -1
	inspect("metadata_invalid")
}
func TestCleanupReviewUnsafeFilesAndPaths(t *testing.T) {
	for _, mode := range []string{"symlink-dump", "symlink-sidecar", "symlink-directory", "hardlink", "outside", "directory"} {
		t.Run(mode, func(t *testing.T) {
			root, e := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, e)
			t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", root)
			a := testArtifact(t, root, "nested/one.dump")
			path := strings.TrimPrefix(a.StorageURI, "file://")
			row := repo.CleanupReviewRow{ID: 1, JobID: 1, JobFound: true, CleanupState: "pending", CleanupToken: uuid.NewString(), StorageURI: a.StorageURI, SizeBytes: a.SizeBytes, Checksum: a.Checksum}
			outside := filepath.Join(root, "original")
			require.NoError(t, os.WriteFile(outside, []byte("do not remove"), 0600))
			switch mode {
			case "symlink-dump":
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.Symlink(outside, path))
			case "symlink-sidecar":
				require.NoError(t, os.Symlink(outside, path+".json"))
			case "symlink-directory":
				require.NoError(t, os.Symlink(filepath.Dir(path), filepath.Join(root, "linked")))
				row.StorageURI = "file://" + filepath.Join(root, "linked/one.dump")
			case "hardlink":
				require.NoError(t, os.Link(path, path+".other"))
			case "outside":
				row.StorageURI = "file://" + filepath.Join(filepath.Dir(root), "outside.dump")
			case "directory":
				require.NoError(t, os.Mkdir(path+".sha256", 0700))
			}
			out, e := inspectCleanupFiles(context.Background(), row)
			require.NoError(t, e)
			require.Equal(t, "unsafe", out.State)
			require.False(t, out.ContentVerified)
			data, e := os.ReadFile(outside)
			require.NoError(t, e)
			require.Equal(t, "do not remove", string(data))
		})
	}
}
func TestCleanupReviewRootAndRedaction(t *testing.T) {
	var s *JobService
	for _, claims := range []*reqctx.CoreXClaims{nil, {UserID: 2}, {UserID: 1, IsRoot: true, Scope: "access", Platforms: []string{"api_key"}}, {UserID: 1, IsRoot: true, Scope: "access", Platforms: []string{"service_actor"}}} {
		ctx := context.Background()
		if claims != nil {
			ctx = reqctx.WithClaims(ctx, claims)
		}
		_, e := s.CleanupReviews(ctx, 1, 20)
		require.ErrorContains(t, e, "BACKUP_ROOT_REQUIRED")
		_, e = s.VerifyCleanupReview(ctx, 1, uuid.NewString(), func(CleanupVerification) error { t.Error("unauthorized read"); return nil })
		require.ErrorContains(t, e, "BACKUP_ROOT_REQUIRED")
	}
	root := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
	_, e := s.CleanupReviews(root, 1, 20)
	require.ErrorContains(t, e, "BACKUP_CLEANUP_REVIEW_UNAVAILABLE")
	for _, e := range []error{errors.New("secret SQL path"), &pgconn.PgError{Code: "42P01", Message: "secret table"}, &pgconn.PgError{Code: "42703", Message: "secret column"}} {
		require.NotContains(t, reviewReadError(e).Error(), "secret")
	}
	for _, row := range []repo.CleanupReviewRow{{CleanupToken: "broken", JobFound: true}, {CleanupToken: uuid.NewString()}, {CleanupToken: uuid.NewString(), JobFound: true, JobDeleted: true}, {CleanupToken: uuid.NewString(), JobFound: true, ArtifactDeleted: true}} {
		require.False(t, cleanupReviewItem(row).Inspectable)
	}
}
