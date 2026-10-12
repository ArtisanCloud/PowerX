package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const capabilityReplacementObservationPurpose = "runtime.recovery.replaceable"

type replacementDirectiveContextKey struct{}

type replacementDirectiveContext struct {
	TaskID         string
	CapabilityUUID uuid.UUID
}

// ContextWithReplacementDirective binds a verifier-selected replacement to a
// capability observation. Both values are typed runtime state; readers never
// derive a replacement from a summary or provider text.
func ContextWithReplacementDirective(ctx context.Context, taskID string, capabilityUUID uuid.UUID) context.Context {
	if ctx == nil || strings.TrimSpace(taskID) == "" || capabilityUUID == uuid.Nil {
		return ctx
	}
	return context.WithValue(ctx, replacementDirectiveContextKey{}, replacementDirectiveContext{TaskID: strings.TrimSpace(taskID), CapabilityUUID: capabilityUUID})
}

// CapabilityMetadataReader exposes only already-frozen capability metadata.
// It never invokes the capability and never returns a credential or endpoint.
type CapabilityMetadataReader struct{}

func (CapabilityMetadataReader) ReadObservation(ctx context.Context, resource ResourceDescriptor, purpose string) (ResourceObservation, error) {
	if resource.Kind != ResourceKindCapability || strings.TrimSpace(purpose) == "" {
		return ResourceObservation{}, fmt.Errorf("capability metadata observation is invalid")
	}
	if !resource.ReadGranted {
		return ResourceObservation{}, ErrResourceReadDenied
	}
	payload, err := json.Marshal(map[string]any{
		"resource_uuid": resource.ResourceUUID.String(), "kind": resource.Kind,
		"display_name": resource.DisplayName, "capability_id": resource.CapabilityID,
		"risk_level": resource.RiskLevel, "classification": resource.Classification,
	})
	if err != nil {
		return ResourceObservation{}, fmt.Errorf("encode capability metadata observation: %w", err)
	}
	observation := ResourceObservation{ResourceUUID: resource.ResourceUUID, Purpose: strings.TrimSpace(purpose), Summary: string(payload)}
	if strings.TrimSpace(purpose) != capabilityReplacementObservationPurpose {
		return observation, nil
	}
	directive, ok := ctx.Value(replacementDirectiveContextKey{}).(replacementDirectiveContext)
	if !ok || directive.TaskID == "" || directive.CapabilityUUID == uuid.Nil {
		return ResourceObservation{}, fmt.Errorf("replacement observation directive is required")
	}
	if resource.CapabilityUUID == uuid.Nil {
		return ResourceObservation{}, fmt.Errorf("observed capability uuid is required")
	}
	allowed := false
	for _, alternativeUUID := range resource.RuntimeContract.AlternativeCapabilityUUIDs {
		if alternativeUUID == directive.CapabilityUUID {
			allowed = true
			break
		}
	}
	if !allowed {
		return ResourceObservation{}, fmt.Errorf("replacement capability is not declared by observed capability contract")
	}
	observation.ReplanDirective = &ReplanDirective{Action: "replace_authorized_capability", CapabilityUUID: directive.CapabilityUUID, TaskID: directive.TaskID}
	return observation, nil
}
