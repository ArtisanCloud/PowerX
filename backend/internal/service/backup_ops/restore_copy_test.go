package backup_ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	pxcrypto "github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestRestoreCopyStoreRejectsTraversalAndSymlinks(t *testing.T) {
	s, err := newRestoreCopyStore(t.TempDir(), "source")
	require.NoError(t, err)
	_, err = s.read("../source")
	require.Equal(t, 400, dto.StatusCode(err))
	id := uuid.NewString()
	target := filepath.Join(t.TempDir(), "foreign.json")
	require.NoError(t, os.WriteFile(target, []byte(`{}`), 0600))
	require.NoError(t, os.Symlink(target, filepath.Join(s.root, id+".json")))
	_, err = s.read(id)
	require.Equal(t, 503, dto.StatusCode(err))
}

func TestRestoreCopyDoesNotExposeEncryptedOwnerPassword(t *testing.T) {
	s, err := newRestoreCopyStore(t.TempDir(), "source")
	require.NoError(t, err)
	id := uuid.NewString()
	record := copyLedger{RestoreCopy: RestoreCopy{ID: id, Status: "success", CreatedAt: time.Now()}, SourceKey: "source", OwnerRole: "private-owner", OwnerPassword: &pxcrypto.Wrapped{CT: "private-encrypted-value"}}
	require.NoError(t, s.save(record))
	info, err := os.Stat(filepath.Join(s.root, id+".json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	rows, err := s.list()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	service := &RestoreCopyService{store: s, ctx: context.Background(), sourceKey: "source", cfg: &pgx.ConnConfig{}}
	_, err = service.List(context.Background())
	require.Equal(t, 403, dto.StatusCode(err))
	public, err := service.List(reqctx.WithIsRoot(context.Background(), true))
	require.NoError(t, err)
	raw, err := json.Marshal(public)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private-owner")
	require.NotContains(t, string(raw), "owner_password")
	require.NotContains(t, string(raw), "private-encrypted-value")
}

func TestRestoreSCRAMUsesVerifierInsteadOfPlaintext(t *testing.T) {
	verifier, err := restoreSCRAM("temporary-secret")
	require.NoError(t, err)
	require.Contains(t, verifier, "SCRAM-SHA-256$4096:")
	require.NotContains(t, verifier, "temporary-secret")
	password, err := restoreRandomPassword()
	require.NoError(t, err)
	require.Len(t, password, 64)
}
