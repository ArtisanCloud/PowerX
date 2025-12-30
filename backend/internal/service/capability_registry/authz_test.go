package capability_registry

import (
	"context"
	"errors"
	"testing"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/stretchr/testify/require"
)

type fakeCapabilityLookup struct {
	record *models.CapabilityRecord
	err    error
}

func (f fakeCapabilityLookup) GetCapability(ctx context.Context, capabilityID string, include bool) (CapabilityRecordView, error) {
	if f.err != nil {
		return CapabilityRecordView{}, f.err
	}
	return CapabilityRecordView{
		Record: f.record,
	}, nil
}

type fakeSafeModeStore struct {
	state SafeModeState
	err   error
}

func (f fakeSafeModeStore) State(ctx context.Context, tenantUUID string) (SafeModeState, error) {
	if f.err != nil {
		return SafeModeState{}, f.err
	}
	return f.state, nil
}

func TestAuthorizationBlocksSafeMode(t *testing.T) {
	authz := NewAuthorizationService(AuthorizationOptions{
		SafeMode: fakeSafeModeStore{
			state: SafeModeState{
				TenantUUID: "tenant-safe",
				Enabled:    true,
				Reason:     "ops",
				UpdatedAt:  time.Now(),
			},
		},
	})

	err := authz.AuthorizeInvocation(context.Background(), CapabilityInvokeRequest{
		TenantUUID: "tenant-safe",
	})
	require.ErrorIs(t, err, ErrSelectorSafeModeActive)
}

func TestAuthorizationRequiresToolGrant(t *testing.T) {
	record := &models.CapabilityRecord{
		CapabilityID: "cap.tool.required",
		Policy:       []byte(`{"visibility":{"tool_grant_ids":{"allow":["grant.alpha"]}}}`),
	}
	authz := NewAuthorizationService(AuthorizationOptions{
		Catalog: fakeCapabilityLookup{record: record},
	})

	err := authz.AuthorizeInvocation(context.Background(), CapabilityInvokeRequest{
		TenantUUID:   "tenant-a",
		CapabilityID: "cap.tool.required",
	})
	require.ErrorIs(t, err, ErrSelectorToolGrantRequired)

	err = authz.AuthorizeInvocation(context.Background(), CapabilityInvokeRequest{
		TenantUUID:   "tenant-a",
		CapabilityID: "cap.tool.required",
		ToolGrantIDs: []string{"grant.alpha"},
	})
	require.NoError(t, err)
}

type failingFeatureFlags struct {
	err error
}

func (f failingFeatureFlags) Allowed(context.Context, string, []string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return false, nil
}

func TestAuthorizationFeatureFlags(t *testing.T) {
	record := &models.CapabilityRecord{
		CapabilityID: "cap.flags",
		Annotations:  []byte(`{"feature_flags":["beta_flag"]}`),
	}
	authz := NewAuthorizationService(AuthorizationOptions{
		Catalog:      fakeCapabilityLookup{record: record},
		FeatureFlags: failingFeatureFlags{},
	})

	err := authz.AuthorizeInvocation(context.Background(), CapabilityInvokeRequest{
		TenantUUID:   "tenant-flags",
		CapabilityID: "cap.flags",
	})
	require.ErrorIs(t, err, ErrSelectorFeatureFlagMissing)

	authz = NewAuthorizationService(AuthorizationOptions{
		Catalog: fakeCapabilityLookup{record: record},
	})
	err = authz.AuthorizeInvocation(context.Background(), CapabilityInvokeRequest{
		TenantUUID:   "tenant-flags",
		CapabilityID: "cap.flags",
		Context: map[string]interface{}{
			"feature_flags": []string{"beta_flag"},
		},
	})
	require.NoError(t, err)

	authz = NewAuthorizationService(AuthorizationOptions{
		Catalog:      fakeCapabilityLookup{record: record},
		FeatureFlags: failingFeatureFlags{err: errors.New("ff error")},
	})
	err = authz.AuthorizeInvocation(context.Background(), CapabilityInvokeRequest{
		TenantUUID:   "tenant-flags",
		CapabilityID: "cap.flags",
	})
	require.Error(t, err)
}
