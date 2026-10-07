package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestResourceSnapshotSeparatesDiscoveryReadAndInvocation(t *testing.T) {
	tenantUUID := uuid.NewString()
	agentUUID := uuid.New()
	resourceUUID := uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, agentUUID, []ResourceDescriptor{{
		ResourceUUID:      resourceUUID,
		Kind:              ResourceKindCapability,
		DisplayName:       "活动报表读取",
		TenantUUID:        tenantUUID,
		CapabilityUUID:    uuid.New(),
		CapabilityID:      "com.example.marketing.report.read",
		DiscoveryGranted:  true,
		ReadGranted:       true,
		InvocationGranted: false,
	}}, time.Now())
	require.NoError(t, err)
	_, err = snapshot.RequireReadable(resourceUUID)
	require.NoError(t, err)
	_, err = snapshot.RequireInvocable(resourceUUID)
	require.ErrorIs(t, err, ErrResourceInvocationDenied)
}

func TestResourceSnapshotRejectsCrossTenantAndUnpublishedDiscovery(t *testing.T) {
	tenantUUID := uuid.NewString()
	_, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{{
		ResourceUUID:     uuid.New(),
		Kind:             ResourceKindCapability,
		DisplayName:      "越权资源",
		TenantUUID:       uuid.NewString(),
		CapabilityUUID:   uuid.New(),
		DiscoveryGranted: true,
	}}, time.Now())
	require.Error(t, err)

	_, err = NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{{
		ResourceUUID:     uuid.New(),
		Kind:             ResourceKindCapability,
		DisplayName:      "未授权资源",
		TenantUUID:       tenantUUID,
		CapabilityUUID:   uuid.New(),
		DiscoveryGranted: false,
	}}, time.Now())
	require.True(t, errors.Is(err, ErrResourceDiscoveryDenied))
}

func TestRuntimeObservationsContextCopiesRedactedResults(t *testing.T) {
	input := []ResourceObservation{{ObservationUUID: uuid.New(), ResourceUUID: uuid.New(), Summary: "redacted", ArtifactRef: "artifact:1"}}
	ctx := ContextWithRuntimeObservations(context.Background(), input)
	got := RuntimeObservationsFromContext(ctx)
	require.Len(t, got, 1)
	got[0].Summary = "mutated"
	require.Equal(t, "redacted", RuntimeObservationsFromContext(ctx)[0].Summary)
}
