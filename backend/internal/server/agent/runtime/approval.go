package runtime

import (
	"context"
	"fmt"
	"strings"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
)

type capabilityApprovalContextKey struct{}

// CapabilityApproval is trusted approval evidence issued by a server-side
// approval workflow. HTTP request data must never be converted to this value.
type CapabilityApproval struct {
	ApprovalUUID   uuid.UUID
	CapabilityUUID uuid.UUID
	EvidenceRef    string
}

// ApprovalRequiredError is a structured control-flow error. It contains no
// user supplied text and is classified by the deterministic verifier.
type ApprovalRequiredError struct {
	CapabilityUUID uuid.UUID
	ApprovalUUID   uuid.UUID
}

func (e *ApprovalRequiredError) Error() string { return "capability approval is required" }

func ContextWithCapabilityApprovals(ctx context.Context, approvals []CapabilityApproval) (context.Context, error) {
	byCapability := make(map[uuid.UUID]CapabilityApproval, len(approvals))
	for _, approval := range approvals {
		if approval.ApprovalUUID == uuid.Nil || approval.CapabilityUUID == uuid.Nil || strings.TrimSpace(approval.EvidenceRef) == "" {
			return nil, fmt.Errorf("capability approval evidence is incomplete")
		}
		if _, exists := byCapability[approval.CapabilityUUID]; exists {
			return nil, fmt.Errorf("duplicate capability approval evidence")
		}
		byCapability[approval.CapabilityUUID] = approval
	}
	return context.WithValue(ctx, capabilityApprovalContextKey{}, byCapability), nil
}

func capabilityApproved(ctx context.Context, capabilityUUID uuid.UUID) bool {
	approvals, _ := ctx.Value(capabilityApprovalContextKey{}).(map[uuid.UUID]CapabilityApproval)
	approval, ok := approvals[capabilityUUID]
	return ok && approval.ApprovalUUID != uuid.Nil && strings.TrimSpace(approval.EvidenceRef) != ""
}

func requireTaskApproval(ctx context.Context, snapshot *ResourceSnapshot, taskCapabilityUUID uuid.UUID) error {
	if snapshot == nil || taskCapabilityUUID == uuid.Nil {
		return nil
	}
	descriptor, err := snapshot.RequireInvocable(taskCapabilityUUID)
	if err != nil {
		return err
	}
	if !descriptor.RuntimeContract.HumanApprovalRequired || capabilityApproved(ctx, taskCapabilityUUID) {
		return nil
	}
	service, ok := CapabilityApprovalServiceFromContext(ctx)
	if !ok {
		return &ApprovalRequiredError{CapabilityUUID: taskCapabilityUUID}
	}
	scope, err := approvalScopeFromRuntime(ctx, snapshot)
	if err != nil {
		return err
	}
	pending, err := service.EnsurePending(ctx, scope, taskCapabilityUUID)
	if err != nil {
		return fmt.Errorf("create capability approval request: %w", err)
	}
	return &ApprovalRequiredError{CapabilityUUID: taskCapabilityUUID, ApprovalUUID: pending.ApprovalUUID}
}

func taskCapabilityUUID(task flowschema.PlanTask) (uuid.UUID, error) {
	if task.Params == nil {
		return uuid.Nil, fmt.Errorf("tooling task capability_uuid is required")
	}
	raw, ok := task.Params["capability_uuid"]
	if !ok {
		return uuid.Nil, fmt.Errorf("tooling task capability_uuid is required")
	}
	value, ok := raw.(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("tooling task capability_uuid must be a UUID string")
	}
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || parsed == uuid.Nil {
		return uuid.Nil, fmt.Errorf("tooling task capability_uuid must be a UUID string")
	}
	return parsed, nil
}
