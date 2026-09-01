package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentsvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	skillsvc "github.com/ArtisanCloud/PowerX/internal/service/skills"
	modelagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
)

type childHandoffRuntime struct {
	teams         *agentsvc.TeamService
	agents        *agentsvc.AgentService
	skillBinds    *agentrepo.AgentSkillBindingRepository
	definitionRun *skillsvc.DefinitionInvokeService
	env           string
}

func newChildHandoffInvoker(runtime childHandoffRuntime) agent.AgentHandoffInvoker {
	return func(ctx context.Context, in agent.AgentHandoffInput) (*agent.AgentHandoffOutput, error) {
		if runtime.teams == nil || runtime.agents == nil || runtime.skillBinds == nil || runtime.definitionRun == nil {
			return nil, fmt.Errorf("agent.handoff_runtime_unavailable")
		}
		tenantUUID := strings.TrimSpace(in.TenantUUID)
		if tenantUUID == "" || in.TeamID == 0 || in.ChildAgentID == 0 || strings.TrimSpace(in.TaskID) == "" {
			return nil, fmt.Errorf("agent.handoff_context_invalid: tenant_uuid, team_id, child_agent_id and task_id are required")
		}

		team, err := runtime.teams.ValidateTeamTenant(ctx, in.TeamID, tenantUUID)
		if err != nil {
			return nil, fmt.Errorf("agent.handoff_team_invalid: %w", err)
		}
		if team.ParentAgentID != in.ParentAgentID {
			return nil, fmt.Errorf("agent.handoff_parent_mismatch: expected=%d actual=%d", team.ParentAgentID, in.ParentAgentID)
		}
		members, err := runtime.teams.ListMembers(ctx, in.TeamID, tenantUUID)
		if err != nil {
			return nil, fmt.Errorf("agent.handoff_members_unavailable: %w", err)
		}
		if !containsEnabledChild(members, in.ChildAgentID) {
			return nil, fmt.Errorf("agent.handoff_child_not_member: child_agent_id=%d", in.ChildAgentID)
		}

		tenantRef := tenantUUID
		child, err := runtime.agents.Get(ctx, runtime.env, &tenantRef, in.ChildAgentID)
		if err != nil {
			return nil, fmt.Errorf("agent.handoff_child_not_found: child_agent_id=%d: %w", in.ChildAgentID, err)
		}
		if !strings.EqualFold(strings.TrimSpace(child.Status), agentmodel.AgentStatusActive) {
			return nil, fmt.Errorf("agent.handoff_child_inactive: child_agent_id=%d status=%s", in.ChildAgentID, child.Status)
		}

		bindings, err := runtime.skillBinds.ListByAgent(ctx, runtime.env, &tenantRef, in.ChildAgentID)
		if err != nil {
			return nil, fmt.Errorf("agent.handoff_skill_bindings_unavailable: %w", err)
		}
		skillID, err := selectHandoffSkill(bindings, in.Payload, in.Context)
		if err != nil {
			return nil, err
		}
		payload := mergeHandoffPayload(in.Context, in.Payload)
		executionContext := mergeHandoffPayload(in.Context, nil)
		executionContext["agent_id"] = in.ChildAgentID
		executionContext["agent_uuid"] = child.UUID.String()
		executionContext["parent_agent_id"] = in.ParentAgentID
		executionContext["team_uuid"] = team.UUID.String()
		executionContext["handoff_task_id"] = in.TaskID

		if err = runtime.teams.CreateHandoffTask(ctx, agentsvc.HandoffRecordInput{
			TaskID: in.TaskID, TeamID: in.TeamID, TenantUUID: tenantUUID,
			ParentAgentID: in.ParentAgentID, ChildAgentID: in.ChildAgentID, SessionID: in.SessionID,
			PlanID: in.PlanID, NodeID: in.NodeID, ContextRef: in.ContextRefID,
			FailurePolicy: in.FailurePolicy, InputPayload: payload, HandoffTraceID: in.HandoffTraceID,
		}); err != nil {
			return nil, fmt.Errorf("agent.handoff_record_create_failed: %w", err)
		}
		if err = runtime.teams.MarkHandoffRunning(ctx, in.TaskID, in.HandoffTraceID); err != nil {
			return nil, fmt.Errorf("agent.handoff_record_start_failed: %w", err)
		}

		executed, err := runtime.definitionRun.Execute(ctx, skillsvc.InvokeRequest{
			TenantUUID: tenantUUID,
			SkillID:    skillID,
			TraceID:    in.HandoffTraceID,
			InvokePath: "agent.team_handoff",
		}, payload, executionContext)
		if err != nil {
			return runtime.failHandoff(ctx, in, err)
		}
		if !strings.EqualFold(strings.TrimSpace(executed.Status), "completed") {
			return runtime.failHandoff(ctx, in, fmt.Errorf("agent.handoff_skill_failed: status=%s", executed.Status))
		}
		if err = runtime.teams.MarkHandoffFinished(ctx, agentsvc.HandoffFinalizeInput{
			TaskID: in.TaskID, Status: "completed", OutputPayload: executed.Result,
		}); err != nil {
			return nil, fmt.Errorf("agent.handoff_record_finish_failed: %w", err)
		}
		return &agent.AgentHandoffOutput{
			TaskID: in.TaskID, HandoffTraceID: in.HandoffTraceID, Status: "completed", Result: executed.Result,
		}, nil
	}
}

func containsEnabledChild(members []modelagent.AgentTeamMember, childAgentID uint64) bool {
	for _, member := range members {
		if member.Enabled && member.ChildAgentID == childAgentID {
			return true
		}
	}
	return false
}

func selectHandoffSkill(bindings []agentmodel.AgentSkillBinding, payload, contextMap map[string]any) (string, error) {
	requested := firstNonEmptyString(asStringFromMap(payload, "child_skill_id"), asStringFromMap(contextMap, "child_skill_id"))
	if requested == "" && len(bindings) != 1 {
		return "", fmt.Errorf("agent.handoff_skill_required: enabled_bindings=%d", len(bindings))
	}
	if requested == "" {
		return strings.TrimSpace(bindings[0].SkillID), nil
	}
	for _, binding := range bindings {
		if strings.EqualFold(strings.TrimSpace(binding.SkillID), requested) {
			return strings.TrimSpace(binding.SkillID), nil
		}
	}
	return "", fmt.Errorf("agent.handoff_skill_not_bound: child_skill_id=%s", requested)
}

func mergeHandoffPayload(contextMap, payload map[string]any) map[string]any {
	out := make(map[string]any, len(contextMap)+len(payload))
	for key, value := range contextMap {
		out[key] = value
	}
	for key, value := range payload {
		out[key] = value
	}
	return out
}

func (runtime childHandoffRuntime) failHandoff(ctx context.Context, in agent.AgentHandoffInput, cause error) (*agent.AgentHandoffOutput, error) {
	_ = runtime.teams.MarkHandoffFinished(ctx, agentsvc.HandoffFinalizeInput{
		TaskID: in.TaskID, Status: "failed", ErrorCode: "agent.handoff_failed", ErrorSummary: cause.Error(),
	})
	return nil, cause
}
