package runtime

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	sessions "github.com/ArtisanCloud/PowerX/internal/service/agent_session"
)

// DurableFinalOutputReader 从已经完成的任务回执恢复唯一终态报告。
type DurableFinalOutputReader struct {
	Runs    TaskRunReader
	Objects agent_run.ReportObjectStore
}

func (r *DurableFinalOutputReader) ReadFinalOutput(ctx context.Context, run agent_run.Snapshot) (sessions.DurableOutput, error) {
	if r == nil || r.Runs == nil || r.Objects == nil || run.Status != "completed" || run.PlanRevision != 1 {
		return sessions.DurableOutput{}, agent_run.ErrInvalid
	}
	planning, err := r.Runs.GetTask(ctx, run, 0, agent_run.PlanningTaskID)
	if err != nil {
		return sessions.DurableOutput{}, err
	}
	if planning.Status != "completed" {
		return sessions.DurableOutput{}, agent_run.ErrConflict
	}
	ref := agent_run.TaskRef{TenantUUID: run.TenantUUID, Env: run.Env, RunID: run.RunID, TaskID: agent_run.PlanningTaskID, Attempt: 1}
	plan, err := LoadExecutionPlanArtifact(ctx, r.Objects, ref, planning.ResultRef, planning.EvidenceRef)
	if err != nil {
		return sessions.DurableOutput{}, err
	}
	if len(plan.Tasks) == 0 {
		return sessions.DurableOutput{}, agent_run.ErrInvalid
	}
	selected := finalDurableTask(*plan)
	state, err := r.Runs.GetTask(ctx, run, run.PlanRevision, selected.TaskID)
	if err != nil {
		return sessions.DurableOutput{}, err
	}
	if state.Status != "completed" {
		return sessions.DurableOutput{}, agent_run.ErrConflict
	}
	ref.TaskID, ref.Revision, ref.Attempt = selected.TaskID, run.PlanRevision, state.Attempt
	out, err := LoadTaskResultArtifact(ctx, r.Objects, ref, state.ResultRef, state.EvidenceRef)
	if err != nil {
		return sessions.DurableOutput{}, err
	}
	envelope, err := responseEnvelopeFromExecutionResultWithTaskRefs(out.Data, finalResponseUpstreamTaskRefs(plan))
	if err != nil {
		return sessions.DurableOutput{}, err
	}
	if envelope != nil {
		if verified, _ := out.Metadata["durable_response_verified"].(bool); !verified {
			return sessions.DurableOutput{}, sessions.ErrDependency
		}
		raw, err := json.Marshal(envelope)
		return sessions.DurableOutput{ResponseEnvelope: raw}, err
	}
	// 无任务意图的默认对话保留既有直接答复；业务执行计划必须返回结构化报告。
	if plan.PlanID != "invoke_default" {
		return sessions.DurableOutput{}, sessions.ErrDependency
	}
	content := SanitizeAssistantVisibleText(buildFinalContent(out))
	if strings.TrimSpace(content) == "" {
		return sessions.DurableOutput{}, sessions.ErrDependency
	}
	return sessions.DurableOutput{Content: content}, nil
}
