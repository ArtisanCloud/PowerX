package runtime

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCapabilityMetadataReaderRequiresReadGrant(t *testing.T) {
	reader := CapabilityMetadataReader{}
	resource := ResourceDescriptor{ResourceUUID: uuid.New(), Kind: ResourceKindCapability, DisplayName: "capability", CapabilityUUID: uuid.New()}
	_, err := reader.ReadObservation(context.Background(), resource, "plan")
	require.ErrorIs(t, err, ErrResourceReadDenied)
	resource.ReadGranted = true
	got, err := reader.ReadObservation(context.Background(), resource, "plan")
	require.NoError(t, err)
	require.Equal(t, resource.ResourceUUID, got.ResourceUUID)
	require.NotContains(t, got.Summary, "invocation_granted")
}

func TestCapabilityMetadataReaderEmitsReplacementDirectiveOnlyFromTypedContext(t *testing.T) {
	primaryUUID := uuid.New()
	replacementUUID := uuid.New()
	reader := CapabilityMetadataReader{}
	resource := ResourceDescriptor{
		ResourceUUID: primaryUUID, Kind: ResourceKindCapability, CapabilityUUID: primaryUUID,
		CapabilityID: "cap.primary", ReadGranted: true,
		RuntimeContract: CapabilityRuntimeContract{AlternativeCapabilityUUIDs: []uuid.UUID{replacementUUID}},
	}
	ctx := ContextWithReplacementDirective(context.Background(), "recover", replacementUUID)
	observation, err := reader.ReadObservation(ctx, resource, capabilityReplacementObservationPurpose)
	require.NoError(t, err)
	require.NotNil(t, observation.ReplanDirective)
	require.Equal(t, "replace_authorized_capability", observation.ReplanDirective.Action)
	require.Equal(t, replacementUUID, observation.ReplanDirective.CapabilityUUID)

	_, err = reader.ReadObservation(context.Background(), resource, capabilityReplacementObservationPurpose)
	require.Error(t, err)
}
