package agent_lifecycle

import (
	"context"
	"fmt"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/google/uuid"
)

func (s *Service) activateTenantForm(ctx context.Context, form *agentmodel.AgentTenantForm, operator string) error {
	toolGrants := decodeToolGrantsJSON(form.ToolGrants)
	metadata := decodeStringMap(form.Metadata)
	registerInput := RegisterInput{
		TenantID:                 form.TenantID,
		Alias:                    form.Alias,
		DisplayName:              form.DisplayName,
		ToolGrants:               toolGrants,
		TelemetryContractVersion: form.TelemetryContractVersion,
		DefaultCapacityInstances: 1,
		MaxCapacityInstances:     nil,
		EventTopicPrefix:         "",
		NotificationChannel:      "",
		Metadata:                 metadata,
		RequestedBy:              operator,
	}
	if registerInput.TelemetryContractVersion == "" {
		registerInput.TelemetryContractVersion = "default"
	}
	result, err := s.Register(ctx, registerInput)
	if err != nil {
		form.Status = tenantFormStatusActivationFailed
		form.LastError = fmt.Sprintf("register failed: %v", err)
		_, _ = s.tenantForms.Save(ctx, form)
		return err
	}

	_, err = s.Activate(ctx, ActivateInput{
		AgentID:     result.Agent.ID,
		TenantID:    form.TenantID,
		Reason:      "tenant-form-approved",
		RequestedBy: operator,
	})
	if err != nil {
		form.Status = tenantFormStatusActivationFailed
		form.LastError = fmt.Sprintf("activate failed: %v", err)
		_, _ = s.tenantForms.Save(ctx, form)
		return err
	}

	form.Status = tenantFormStatusActivated
	form.LastError = ""
	form.ActivatedAgentID = ptrUUID(result.Agent.ID)
	if _, err := s.tenantForms.Save(ctx, form); err != nil {
		return err
	}
	return nil
}

func ptrUUID(id uuid.UUID) *uuid.UUID {
	return &id
}
