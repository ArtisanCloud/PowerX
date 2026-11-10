package archive_ops

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	vault "github.com/ArtisanCloud/PowerX/internal/infra/archive_credentials"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func credentialRootContext() context.Context {
	return reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 11, IsRoot: true, Scope: "access"})
}
func attachOwnedCredentialVault(t *testing.T, s *Service) (vault.Config, vault.Keyring) {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	dir := filepath.Join(root, "vault")
	require.NoError(t, os.Mkdir(dir, 0700))
	key := make([]byte, 32)
	_, e = rand.Read(key)
	require.NoError(t, e)
	c := vault.Config{Directory: dir, KeyringFile: filepath.Join(root, "external-keyring.json"), StoreID: "owned-archive-vault"}
	k := vault.Keyring{Version: 1, StoreID: c.StoreID, ActiveKeyID: "key1", Keys: map[string]string{"key1": base64.StdEncoding.EncodeToString(key)}}
	raw, e := json.Marshal(k)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(c.KeyringFile, raw, 0600))
	s.cfg.CredentialVault = &c
	s.initCredentialVault()
	require.NoError(t, s.vaultErr)
	return c, k
}
func TestCredentialVaultAdoptionInvalidatesTargetAndRevocationNeverFallsBack(t *testing.T) {
	s, _, target, _ := s3Fixture(t)
	ctx := credentialRootContext()
	c, k := attachOwnedCredentialVault(t, s)
	require.NoError(t, s.TestTarget(ctx, target.ID))
	target, e := s.target(target.ID)
	require.NoError(t, e)
	require.True(t, s.targetVerified(target))
	before := s.cfg.S3Credentials[target.CredentialRef]
	overview, e := s.CredentialOverview(ctx)
	require.NoError(t, e)
	require.True(t, overview.Store.Ready)
	var row CredentialView
	for _, v := range overview.Items {
		if v.Ref == target.CredentialRef {
			row = v
		}
	}
	require.Equal(t, "legacy", row.Origin)
	in := CredentialControlInput{RequestID: uuid.NewString(), ExpectedRevision: row.Revision, LegacyDigest: row.LegacyDigest, Confirm: true}
	saved, e := s.AdoptS3Credential(ctx, row.Ref, in)
	require.NoError(t, e)
	require.EqualValues(t, 1, saved.Receipt.Revision)
	require.False(t, s.targetVerified(target))
	require.Equal(t, before, s.cfg.S3Credentials[row.Ref])
	replay, e := s.AdoptS3Credential(ctx, row.Ref, in)
	require.NoError(t, e)
	require.Equal(t, saved.Receipt, replay.Receipt)
	receipt, e := s.CredentialRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, saved.Receipt, receipt.Receipt)
	raw, e := os.ReadFile(filepath.Join(c.Directory, "credentials.sealed.json"))
	require.NoError(t, e)
	require.NotContains(t, string(raw), before.SecretKey)
	_, e = s.RevokeS3Credential(ctx, row.Ref, CredentialControlInput{RequestID: uuid.NewString(), ExpectedRevision: 1, Confirm: true})
	require.NoError(t, e)
	_, e = s.s3Credential(row.Ref)
	require.Error(t, e)
	refs, e := s.s3CredentialRefs()
	require.NoError(t, e)
	require.NotContains(t, refs, row.Ref)
	s.cfg.CredentialVault = nil
	s.vault = nil
	_, e = s.s3Credential(row.Ref)
	require.Error(t, e, "missing provider cannot revive an old deployment credential")
	s.cfg.CredentialVault = &c
	s.initCredentialVault()
	delete(k.Keys, "key1")
	require.NoError(t, os.WriteFile(c.KeyringFile, []byte(`{}`), 0600))
	_, e = s.s3Credential(row.Ref)
	require.Error(t, e)
}
func TestCredentialVaultSaveCASUnknownReceiptAndRootPlane(t *testing.T) {
	s, _, _, _ := s3Fixture(t)
	ctx := credentialRootContext()
	c, _ := attachOwnedCredentialVault(t, s)
	in := SaveS3CredentialInput{RequestID: uuid.NewString(), Ref: "new-private", AccessKey: "owned-only-access", SecretKey: "owned-only-secret"}
	saved, e := s.SaveS3Credential(ctx, in)
	require.NoError(t, e)
	require.EqualValues(t, 1, saved.Receipt.Revision)
	require.NoFileExists(t, filepath.Join(s.cfg.Directory, "secrets", in.Ref+".json"))
	_, e = s.SaveS3Credential(ctx, in)
	require.NoError(t, e)
	changed := in
	changed.SecretKey = "other"
	_, e = s.SaveS3Credential(ctx, changed)
	require.Error(t, e)
	in.ExpectedRevision = 1
	in.RequestID = uuid.NewString()
	_, e = s.SaveS3Credential(ctx, in)
	require.NoError(t, e)
	for _, claims := range []reqctx.CoreXClaims{{UserID: 11}, {UserID: 11, IsRoot: true, Platforms: []string{"api_key"}}, {UserID: 11, IsRoot: true, Scope: "service"}, {IsRoot: true}} {
		bad := reqctx.WithClaims(context.Background(), &claims)
		_, e = s.CredentialOverview(bad)
		require.Error(t, e)
		_, e = s.CredentialRequest(bad, in.RequestID)
		require.Error(t, e)
	}
	raw, e := json.Marshal(saved)
	require.NoError(t, e)
	require.NotContains(t, string(raw), in.SecretKey)
	require.NotContains(t, string(raw), in.AccessKey)
	// 仅复制密文无法恢复；主密钥移除后不使用任何旧明文回退。
	require.NoError(t, os.Remove(c.KeyringFile))
	_, e = s.s3Credential(in.Ref)
	require.Error(t, e)
}

func TestCredentialVaultInvalidInputDoesNotBindAndRuntimeSnapshotContainsReferencesOnly(t *testing.T) {
	s, _, _, _ := s3Fixture(t)
	attachOwnedCredentialVault(t, s)
	ctx := credentialRootContext()
	_, e := s.SaveS3Credential(ctx, SaveS3CredentialInput{Ref: "invalid", AccessKey: "x", SecretKey: "x"})
	require.Error(t, e)
	require.NoFileExists(t, s.credentialBindingPath())
	t.Setenv("POWERX_OPS_ARCHIVE_CONFIG", "")
	require.NoError(t, s.PublishRuntimeConfig())
	raw, e := os.ReadFile(filepath.Join(s.cfg.Directory, "runtime-config.yaml"))
	require.NoError(t, e)
	require.NotContains(t, string(raw), "fixture-secret")
	require.NotContains(t, string(raw), "fixture-access")
	require.Contains(t, string(raw), "credential_vault")
}

func TestCredentialVaultMissingOrRolledBackCiphertextCannotReviveLegacyCredentials(t *testing.T) {
	s, _, target, _ := s3Fixture(t)
	ctx := credentialRootContext()
	c, _ := attachOwnedCredentialVault(t, s)
	view, e := s.CredentialOverview(ctx)
	require.NoError(t, e)
	var old CredentialView
	for _, r := range view.Items {
		if r.Ref == target.CredentialRef {
			old = r
		}
	}
	_, e = s.AdoptS3Credential(ctx, old.Ref, CredentialControlInput{RequestID: uuid.NewString(), ExpectedRevision: old.Revision, LegacyDigest: old.LegacyDigest, Confirm: true})
	require.NoError(t, e)
	path := filepath.Join(c.Directory, "credentials.sealed.json")
	before, e := os.ReadFile(path)
	require.NoError(t, e)
	_, e = s.RevokeS3Credential(ctx, old.Ref, CredentialControlInput{RequestID: uuid.NewString(), ExpectedRevision: 1, Confirm: true})
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(path, before, 0600))
	_, e = s.s3Credential(old.Ref)
	require.Error(t, e, "old ciphertext cannot bypass the independent minimum credential revision")
	view, e = s.CredentialOverview(ctx)
	require.NoError(t, e)
	require.False(t, view.Store.Ready)
	for _, r := range view.Items {
		require.Equal(t, "unavailable", r.State)
	}
	require.NoError(t, os.Remove(path))
	_, e = s.s3Credential(old.Ref)
	require.Error(t, e, "a missing encrypted ledger must never fall back to plaintext")
	_, e = s.SaveS3Credential(ctx, SaveS3CredentialInput{RequestID: uuid.NewString(), Ref: "new", AccessKey: "new-access", SecretKey: "new-secret"})
	require.Error(t, e, "cannot replace a lost authority with an empty ledger")
}
