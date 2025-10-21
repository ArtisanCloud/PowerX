package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
)

// ToolGrantValidator 定义派发前校验 Tool Grant 的接口。
type ToolGrantValidator interface {
	ValidateAgentGrant(ctx context.Context, tenantID uint64, agentID uuid.UUID, capability string) (GrantValidationResult, error)
}

// GrantValidationResult 描述 Grant 校验返回的关键信息。
type GrantValidationResult struct {
	GrantID        string
	GrantVersion   int64
	CapabilityName string
}

// AssignmentDispatchInput 描述一次派发的上下文。
type AssignmentDispatchInput struct {
	TenantID     uint64
	InstanceUUID uuid.UUID
	StepRecordID uint64
	StepID       string
	AgentUUID    uuid.UUID
	Capability   string
}

// TimeoutProcessOptions 描述超时扫描的参数。
type TimeoutProcessOptions struct {
	TenantID uint64
	Limit    int
}

// TimeoutProcessResult 返回处理超时派发后的结果。
type TimeoutProcessResult struct {
	TimedOutAssignments []modelworkflow.AgentAssignment
}

// AssignmentTrackerOptions 构建 AssignmentTracker 所需依赖。
type AssignmentTrackerOptions struct {
	AssignmentStore AssignmentStore
	StepStore       StepRecordStore
	GrantValidator  ToolGrantValidator
	Clock           func() time.Time
	AckTimeout      time.Duration
}

// AssignmentTracker 负责 Agent 派发生命周期管理与超时重试。
type AssignmentTracker struct {
	assignments AssignmentStore
	steps       StepRecordStore
	validator   ToolGrantValidator
	now         func() time.Time
	ackTimeout  time.Duration
}

// NewAssignmentTracker 创建派发跟踪器。
func NewAssignmentTracker(opts AssignmentTrackerOptions) *AssignmentTracker {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	ack := opts.AckTimeout
	if ack <= 0 {
		ack = 2 * time.Minute
	}
	if opts.AssignmentStore == nil || opts.StepStore == nil {
		return nil
	}
	return &AssignmentTracker{
		assignments: opts.AssignmentStore,
		steps:       opts.StepStore,
		validator:   opts.GrantValidator,
		now:         clock,
		ackTimeout:  ack,
	}
}

// Dispatch 在派发步骤前校验 Tool Grant 并更新步骤状态。
func (t *AssignmentTracker) Dispatch(ctx context.Context, input AssignmentDispatchInput) (*modelworkflow.AgentAssignment, error) {
	if t == nil || t.assignments == nil || t.steps == nil {
		return nil, errors.New("assignment tracker unavailable")
	}
	if input.TenantID == 0 {
		return nil, errors.New("tenant_id is required")
	}
	if input.InstanceUUID == uuid.Nil {
		return nil, errors.New("instance uuid is required")
	}
	if input.StepRecordID == 0 {
		return nil, errors.New("step record id is required")
	}
	if input.AgentUUID == uuid.Nil {
		return nil, errors.New("agent uuid is required")
	}
	step, err := t.steps.GetByID(ctx, input.StepRecordID)
	if err != nil {
		return nil, err
	}
	if step.InstanceUUID != input.InstanceUUID {
		return nil, fmt.Errorf("step %d does not belong to instance %s", step.ID, input.InstanceUUID)
	}

	var grantResult GrantValidationResult
	if t.validator != nil {
		grantResult, err = t.validator.ValidateAgentGrant(ctx, input.TenantID, input.AgentUUID, input.Capability)
		if err != nil {
			return nil, err
		}
	}

	attempt := step.Attempt
	if attempt <= 0 {
		attempt = 1
	}
	now := t.now().UTC()

	updates := map[string]any{
		"subject_uuid":       input.AgentUUID,
		"subject_type":       "agent",
		"tool_grant_id":      grantResult.GrantID,
		"tool_grant_version": grantResult.GrantVersion,
		"attempt":            attempt,
		"started_at":         now,
		"awaiting_human":     false,
		"failure_reason":     "",
	}
	if err := t.steps.UpdateState(ctx, step.ID, "in_progress", updates); err != nil {
		return nil, err
	}

	deadline := now.Add(t.ackTimeout)
	assignment := &modelworkflow.AgentAssignment{
		StepRecordID: step.ID,
		AgentUUID:    input.AgentUUID,
		Status:       "dispatched",
		DispatchedAt: now,
		AckDeadline:  &deadline,
	}
	created, err := t.assignments.CreateAssignment(ctx, assignment)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// ProcessTimeouts 扫描并标记超时派发，返回需要重试的步骤。
func (t *AssignmentTracker) ProcessTimeouts(ctx context.Context, opts TimeoutProcessOptions) (TimeoutProcessResult, error) {
	result := TimeoutProcessResult{}
	if t == nil || t.assignments == nil || t.steps == nil {
		return result, errors.New("assignment tracker unavailable")
	}
	if opts.TenantID == 0 {
		return result, errors.New("tenant_id is required")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	now := t.now().UTC()
	assignments, err := t.assignments.FindTimedOutAssignments(ctx, opts.TenantID, now, limit)
	if err != nil {
		return result, err
	}
	if len(assignments) == 0 {
		return result, nil
	}

	for _, assignment := range assignments {
		step, err := t.steps.GetByID(ctx, assignment.StepRecordID)
		if err != nil {
			return result, err
		}
		if step.InstanceUUID == uuid.Nil {
			return result, fmt.Errorf("step %d missing instance reference", step.ID)
		}

		attempt := step.Attempt
		if attempt <= 0 {
			attempt = 1
		} else {
			attempt++
		}

		updates := map[string]any{
			"attempt":        attempt,
			"failure_reason": "assignment timeout",
		}
		if err := t.steps.UpdateState(ctx, step.ID, "queued", updates); err != nil {
			return result, err
		}

		if err := t.assignments.UpdateStatus(ctx, assignment.ID, "timeout", map[string]any{
			"completed_at": now,
		}); err != nil {
			return result, err
		}

		result.TimedOutAssignments = append(result.TimedOutAssignments, assignment)
	}

	return result, nil
}
