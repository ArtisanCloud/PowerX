package media

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type MediaUploadRequest struct {
	TenantID         string
	OperatorID       string
	Name             string
	Driver           string
	UploadMethod     string
	ExternalURL      string
	Tags             []string
	PresignTokenID   string
	PresignExpiresIn int64
}

type mediaIntegrationTestEnv struct {
	t *testing.T
}

func newMediaIntegrationTestEnv(t *testing.T) *mediaIntegrationTestEnv {
	t.Helper()
	return &mediaIntegrationTestEnv{t: t}
}

func (env *mediaIntegrationTestEnv) UploadDraftAsset(ctx context.Context, req MediaUploadRequest) (string, error) {
	return "", errors.New("media upload flow not implemented")
}

func (env *mediaIntegrationTestEnv) TriggerProcessingFailure(ctx context.Context, assetID string) error {
	return errors.New("processing failure simulation not implemented")
}

func (env *mediaIntegrationTestEnv) LookupAsset(ctx context.Context, assetID string) (any, error) {
	return nil, ErrMediaAssetNotFound
}

func (env *mediaIntegrationTestEnv) CollectAuditEvents() []string {
	return nil
}

func TestMediaAssetUploadFlowRollbackOnFailure(t *testing.T) {
	t.Parallel()

	env := newMediaIntegrationTestEnv(t)
	ctx := context.Background()

	request := MediaUploadRequest{
		TenantID:       "tenant_a",
		OperatorID:     "op_admin",
		Name:           "homepage-banner",
		Driver:         "local",
		UploadMethod:   "direct_upload",
		Tags:           []string{"banner", "homepage"},
		PresignTokenID: "token_123",
	}

	assetID, err := env.UploadDraftAsset(ctx, request)
	require.NoError(t, err)
	require.NotEmpty(t, assetID)

	err = env.TriggerProcessingFailure(ctx, assetID)
	require.NoError(t, err)

	_, lookupErr := env.LookupAsset(ctx, assetID)
	require.Error(t, lookupErr)
	require.ErrorIs(t, lookupErr, ErrMediaAssetNotFound)

	events := env.CollectAuditEvents()
	require.Len(t, events, 2)
	require.Equal(t, "media.asset.rollback", events[1])
}

var ErrMediaAssetNotFound = errors.New("media asset not found")
