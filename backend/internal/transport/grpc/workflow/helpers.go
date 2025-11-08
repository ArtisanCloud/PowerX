package workflow

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	workflowv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/workflow/v1"
	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/datatypes"
)

func tenantIDFromContext(ctx *commonv1.RequestContext) uint64 {
	if ctx == nil || ctx.GetTenantId() <= 0 {
		return 0
	}
	return uint64(ctx.GetTenantId())
}

func memberUUIDFromContext(ctx *commonv1.RequestContext) uuid.UUID {
	if ctx == nil || ctx.GetMemberId() == 0 {
		return uuid.New()
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(strconv.FormatInt(ctx.GetMemberId(), 10)))
}

func pbStepsToInternal(defs []*workflowv1.WorkflowStepDefinition) []workflowsvc.StepDefinition {
	if len(defs) == 0 {
		return nil
	}
	steps := make([]workflowsvc.StepDefinition, 0, len(defs))
	for _, pb := range defs {
		if pb == nil {
			continue
		}
		config := structToMap(pb.GetConfig())
		steps = append(steps, workflowsvc.StepDefinition{
			ID:            pb.GetStepId(),
			DisplayName:   pb.GetDisplayName(),
			Type:          stepTypeString(pb.GetType()),
			Config:        config,
			NextStepIDs:   pb.GetNextStepIds(),
			DependsOn:     pb.GetDependsOn(),
			Compensatable: pb.GetCompensatable(),
		})
	}
	return steps
}

func stepTypeString(t workflowv1.StepType) string {
	switch t {
	case workflowv1.StepType_STEP_TYPE_AGENT:
		return "agent"
	case workflowv1.StepType_STEP_TYPE_SYSTEM:
		return "system"
	case workflowv1.StepType_STEP_TYPE_DECISION:
		return "decision"
	case workflowv1.StepType_STEP_TYPE_PARALLEL:
		return "parallel"
	case workflowv1.StepType_STEP_TYPE_HUMAN_APPROVAL:
		return "human_approval"
	case workflowv1.StepType_STEP_TYPE_COMPENSATION:
		return "compensation"
	default:
		return strings.ToLower(t.String())
	}
}

func modelDefinitionToPB(def *modelworkflow.WorkflowDefinition) *workflowv1.WorkflowDefinition {
	if def == nil {
		return nil
	}

	steps := decodeSteps(def.StepGraph)
	pbSteps := make([]*workflowv1.WorkflowStepDefinition, 0, len(steps))
	for _, step := range steps {
		cfg, _ := structpb.NewStruct(step.Config)
		pbSteps = append(pbSteps, &workflowv1.WorkflowStepDefinition{
			StepId:        step.ID,
			DisplayName:   step.DisplayName,
			Type:          workflowStepType(step.Type),
			Config:        cfg,
			Compensatable: step.Compensatable,
			NextStepIds:   step.NextStepIDs,
			DependsOn:     step.DependsOn,
		})
	}

	return &workflowv1.WorkflowDefinition{
		DefinitionId:       def.UUID.String(),
		TenantId:           strconv.FormatUint(def.TenantID, 10),
		Name:               def.Name,
		Description:        def.Description,
		Version:            def.Version,
		Status:             workflowDefinitionStatus(def.Status),
		DefaultRetryPolicy: jsonToRetryPolicy(def.DefaultRetryPolicy),
		CompensationPolicy: jsonToCompensationPolicy(def.CompensationPolicy),
		SlaPolicy:          jsonToSlaPolicy(def.SlaPolicy),
		Steps:              pbSteps,
		Metadata:           jsonToStruct(def.Metadata),
		CreatedAt:          timestamppb.New(def.CreatedAt),
		UpdatedAt:          timestamppb.New(def.UpdatedAt),
		PublishedAt:        timestampOrNil(def.PublishedAt),
		CreatedBy:          def.CreatedBy.String(),
	}
}

func modelInstanceToPB(instance *modelworkflow.WorkflowInstance, records []modelworkflow.WorkflowStepRecord) *workflowv1.WorkflowInstance {
	if instance == nil {
		return nil
	}

	pb := &workflowv1.WorkflowInstance{
		InstanceId:        instance.UUID.String(),
		TenantId:          strconv.FormatUint(instance.TenantID, 10),
		DefinitionId:      instance.DefinitionUUID.String(),
		DefinitionVersion: instance.DefinitionVersion,
		State:             workflowInstanceState(instance.State),
		Input:             jsonToStruct(instance.InputContext),
		Output:            jsonToStruct(instance.OutputContext),
		LastError:         instance.LastError,
		CreatedAt:         timestamppb.New(instance.CreatedAt),
		UpdatedAt:         timestamppb.New(instance.UpdatedAt),
		Tags:              jsonToStringMap(instance.Tags),
	}
	if instance.StartedAt != nil {
		pb.StartedAt = timestamppb.New(*instance.StartedAt)
	}
	if instance.CompletedAt != nil {
		pb.CompletedAt = timestamppb.New(*instance.CompletedAt)
	}

	if len(records) > 0 {
		pbSteps := make([]*workflowv1.WorkflowStepSummary, 0, len(records))
		for _, rec := range records {
			pbSteps = append(pbSteps, &workflowv1.WorkflowStepSummary{
				StepRecordId:           strconv.FormatUint(rec.ID, 10),
				StepId:                 rec.StepID,
				Type:                   workflowStepType(rec.Type),
				State:                  workflowStepState(rec.State),
				SubjectType:            stepSubjectType(rec.SubjectType),
				SubjectId:              rec.SubjectUUID.String(),
				ToolGrantVersion:       rec.ToolGrantVer,
				Attempt:                rec.Attempt,
				ScheduledAt:            timestamppb.New(rec.ScheduledAt),
				PayloadIn:              jsonToStruct(rec.PayloadIn),
				PayloadOut:             jsonToStruct(rec.PayloadOut),
				FailureReason:          rec.FailureReason,
				AwaitingManualApproval: rec.AwaitingHuman,
			})
			if rec.StartedAt != nil {
				pbSteps[len(pbSteps)-1].StartedAt = timestamppb.New(*rec.StartedAt)
			}
			if rec.CompletedAt != nil {
				pbSteps[len(pbSteps)-1].CompletedAt = timestamppb.New(*rec.CompletedAt)
			}
		}
		pb.Steps = pbSteps
	}

	return pb
}

func retryPolicyToMap(policy *workflowv1.RetryPolicy) map[string]any {
	if policy == nil {
		return nil
	}
	return map[string]any{
		"max_attempts":        policy.GetMaxAttempts(),
		"initial_interval_ms": policy.GetInitialIntervalMs(),
		"backoff_multiplier":  policy.GetBackoffMultiplier(),
		"max_interval_ms":     policy.GetMaxIntervalMs(),
		"jitter_ms":           policy.GetJitterMs(),
	}
}

func compensationPolicyToMap(policy *workflowv1.CompensationPolicy) map[string]any {
	if policy == nil {
		return nil
	}
	return map[string]any{
		"enabled":                 policy.GetEnabled(),
		"require_manual_approval": policy.GetRequireManualApproval(),
		"max_concurrent":          policy.GetMaxConcurrent(),
		"escalation_channel":      policy.GetEscalationChannel(),
	}
}

func slaPolicyToMap(policy *workflowv1.SlaPolicy) map[string]any {
	if policy == nil {
		return nil
	}
	return map[string]any{
		"step_timeout_seconds":       policy.GetStepTimeoutSeconds(),
		"overall_timeout_seconds":    policy.GetOverallTimeoutSeconds(),
		"heartbeat_interval_seconds": policy.GetHeartbeatIntervalSeconds(),
	}
}

func workflowStepType(stepType string) workflowv1.StepType {
	switch strings.ToLower(stepType) {
	case "agent":
		return workflowv1.StepType_STEP_TYPE_AGENT
	case "system":
		return workflowv1.StepType_STEP_TYPE_SYSTEM
	case "decision":
		return workflowv1.StepType_STEP_TYPE_DECISION
	case "parallel":
		return workflowv1.StepType_STEP_TYPE_PARALLEL
	case "human_approval":
		return workflowv1.StepType_STEP_TYPE_HUMAN_APPROVAL
	case "compensation":
		return workflowv1.StepType_STEP_TYPE_COMPENSATION
	default:
		return workflowv1.StepType_STEP_TYPE_UNSPECIFIED
	}
}

func stepSubjectType(subject string) workflowv1.StepSubjectType {
	switch strings.ToLower(subject) {
	case "system":
		return workflowv1.StepSubjectType_STEP_SUBJECT_TYPE_SYSTEM
	case "agent":
		return workflowv1.StepSubjectType_STEP_SUBJECT_TYPE_AGENT
	case "human":
		return workflowv1.StepSubjectType_STEP_SUBJECT_TYPE_HUMAN
	default:
		return workflowv1.StepSubjectType_STEP_SUBJECT_TYPE_UNSPECIFIED
	}
}

func workflowStepState(state string) workflowv1.WorkflowStepState {
	switch strings.ToLower(state) {
	case "queued":
		return workflowv1.WorkflowStepState_WORKFLOW_STEP_STATE_QUEUED
	case "in_progress":
		return workflowv1.WorkflowStepState_WORKFLOW_STEP_STATE_IN_PROGRESS
	case "waiting":
		return workflowv1.WorkflowStepState_WORKFLOW_STEP_STATE_WAITING
	case "completed":
		return workflowv1.WorkflowStepState_WORKFLOW_STEP_STATE_COMPLETED
	case "failed":
		return workflowv1.WorkflowStepState_WORKFLOW_STEP_STATE_FAILED
	case "compensating":
		return workflowv1.WorkflowStepState_WORKFLOW_STEP_STATE_COMPENSATING
	case "compensated":
		return workflowv1.WorkflowStepState_WORKFLOW_STEP_STATE_COMPENSATED
	default:
		return workflowv1.WorkflowStepState_WORKFLOW_STEP_STATE_UNSPECIFIED
	}
}

func workflowDefinitionStatus(status string) workflowv1.WorkflowDefinitionStatus {
	switch strings.ToLower(status) {
	case "draft":
		return workflowv1.WorkflowDefinitionStatus_WORKFLOW_DEFINITION_STATUS_DRAFT
	case "published":
		return workflowv1.WorkflowDefinitionStatus_WORKFLOW_DEFINITION_STATUS_PUBLISHED
	case "archived":
		return workflowv1.WorkflowDefinitionStatus_WORKFLOW_DEFINITION_STATUS_ARCHIVED
	default:
		return workflowv1.WorkflowDefinitionStatus_WORKFLOW_DEFINITION_STATUS_UNSPECIFIED
	}
}

func workflowInstanceState(state string) workflowv1.WorkflowInstanceState {
	switch strings.ToLower(state) {
	case "draft":
		return workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_DRAFT
	case "running":
		return workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_RUNNING
	case "waiting":
		return workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_WAITING
	case "suspended":
		return workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_SUSPENDED
	case "succeeded":
		return workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_SUCCEEDED
	case "failed":
		return workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_FAILED
	case "compensating":
		return workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_COMPENSATING
	case "compensated":
		return workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_COMPENSATED
	case "canceled":
		return workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_CANCELED
	case "compensation_failed":
		return workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_COMPENSATION_FAILED
	default:
		return workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_UNSPECIFIED
	}
}

func jsonToStruct(data datatypes.JSON) *structpb.Struct {
	if len(data) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil
	}
	st, err := structpb.NewStruct(obj)
	if err != nil {
		return nil
	}
	return st
}

func jsonToStringMap(data datatypes.JSON) map[string]string {
	if len(data) == 0 {
		return nil
	}
	var obj map[string]string
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil
	}
	return obj
}

func timestampOrNil(ts *time.Time) *timestamppb.Timestamp {
	if ts == nil || ts.IsZero() {
		return nil
	}
	return timestamppb.New(*ts)
}

func decodeSteps(data datatypes.JSON) []workflowsvc.StepDefinition {
	if len(data) == 0 {
		return nil
	}
	var steps []workflowsvc.StepDefinition
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil
	}
	return steps
}

func jsonToRetryPolicy(data datatypes.JSON) *workflowv1.RetryPolicy {
	if len(data) == 0 {
		return nil
	}
	raw := make(map[string]any)
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	policy := &workflowv1.RetryPolicy{
		MaxAttempts:       int32(toInt64(raw["max_attempts"])),
		InitialIntervalMs: toInt64(raw["initial_interval_ms"]),
		BackoffMultiplier: toFloat64(raw["backoff_multiplier"]),
		MaxIntervalMs:     toInt64(raw["max_interval_ms"]),
		JitterMs:          toInt64(raw["jitter_ms"]),
	}
	if policy.MaxAttempts == 0 && policy.InitialIntervalMs == 0 && policy.BackoffMultiplier == 0 && policy.MaxIntervalMs == 0 && policy.JitterMs == 0 {
		return nil
	}
	return policy
}

func jsonToCompensationPolicy(data datatypes.JSON) *workflowv1.CompensationPolicy {
	if len(data) == 0 {
		return nil
	}
	raw := make(map[string]any)
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	policy := &workflowv1.CompensationPolicy{
		Enabled:               toBool(raw["enabled"]),
		RequireManualApproval: toBool(raw["require_manual_approval"]),
		MaxConcurrent:         int32(toInt64(raw["max_concurrent"])),
		EscalationChannel:     toString(raw["escalation_channel"]),
	}
	if !policy.Enabled && !policy.RequireManualApproval && policy.MaxConcurrent == 0 && policy.EscalationChannel == "" {
		return nil
	}
	return policy
}

func jsonToSlaPolicy(data datatypes.JSON) *workflowv1.SlaPolicy {
	if len(data) == 0 {
		return nil
	}
	raw := make(map[string]any)
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	policy := &workflowv1.SlaPolicy{
		StepTimeoutSeconds:       toInt64(raw["step_timeout_seconds"]),
		OverallTimeoutSeconds:    toInt64(raw["overall_timeout_seconds"]),
		HeartbeatIntervalSeconds: toInt64(raw["heartbeat_interval_seconds"]),
	}
	if policy.StepTimeoutSeconds == 0 && policy.OverallTimeoutSeconds == 0 && policy.HeartbeatIntervalSeconds == 0 {
		return nil
	}
	return policy
}

func structToMap(st *structpb.Struct) map[string]any {
	if st == nil {
		return nil
	}
	return st.AsMap()
}

func structToStringMap(st *structpb.Struct) map[string]string {
	if st == nil {
		return nil
	}
	raw := st.AsMap()
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		result[k] = toString(v)
	}
	return result
}

func toInt64(v any) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int32:
		return int64(val)
	case float64:
		return int64(val)
	case json.Number:
		i, _ := val.Int64()
		return i
	case string:
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return 0
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return 0
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		b, _ := strconv.ParseBool(val)
		return b
	default:
		return false
	}
}

func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(val, 10)
	case int32:
		return strconv.FormatInt(int64(val), 10)
	case bool:
		return strconv.FormatBool(val)
	case json.Number:
		return val.String()
	default:
		bytes, _ := json.Marshal(val)
		return string(bytes)
	}
}
