package runtime

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// CapabilityRuntimeContract is frozen with a ResourceDescriptor. It is derived
// from the published Capability policy, never from executor result metadata.
type CapabilityRuntimeContract struct {
	VerificationRequired       bool        `json:"verification_required,omitempty"`
	SideEffectEvidenceSchema   string      `json:"side_effect_evidence_schema,omitempty"`
	BusinessCompletionRequired bool        `json:"business_completion_required,omitempty"`
	RetryMaxAttempts           int         `json:"retry_max_attempts,omitempty"`
	AlternativeCapabilityIDs   []string    `json:"alternative_capability_ids,omitempty"`
	AlternativeCapabilityUUIDs []uuid.UUID `json:"alternative_capability_uuids,omitempty"`
	HumanApprovalRequired      bool        `json:"human_approval_required,omitempty"`
}

type capabilityPolicyPayload struct {
	RuntimeContract CapabilityRuntimeContract `json:"runtime_contract"`
}

func ParseCapabilityRuntimeContract(raw []byte) (CapabilityRuntimeContract, error) {
	if len(raw) == 0 {
		return CapabilityRuntimeContract{}, nil
	}
	var policy capabilityPolicyPayload
	if err := json.Unmarshal(raw, &policy); err != nil {
		return CapabilityRuntimeContract{}, fmt.Errorf("decode capability runtime contract: %w", err)
	}
	contract := policy.RuntimeContract
	if contract.RetryMaxAttempts < 0 || contract.RetryMaxAttempts > 1 {
		return CapabilityRuntimeContract{}, fmt.Errorf("capability retry_max_attempts is invalid")
	}
	if (contract.HumanApprovalRequired || contract.VerificationRequired) && strings.TrimSpace(contract.SideEffectEvidenceSchema) == "" {
		return CapabilityRuntimeContract{}, fmt.Errorf("capability side_effect_evidence_schema is required for approval or verification")
	}
	if contract.BusinessCompletionRequired && !contract.VerificationRequired {
		return CapabilityRuntimeContract{}, fmt.Errorf("business_completion_required requires verification_required")
	}
	return contract, nil
}
