package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	trace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
)

// BuildReport 从权威调度状态生成原追踪界面的合同，不依赖本机打印日志。
func (s *AdminRunService) BuildReport(ctx context.Context, query trace.AgentReportQuery) (*trace.AgentRunReport, error) {
	row, err := s.AuthorizedRun(ctx, query.RunID)
	if err != nil {
		return nil, err
	}
	if query.TenantUUID != row.TenantUUID.String() || (query.SessionID != strconv.FormatUint(row.SessionID, 10) && query.SessionID != row.SessionUUID.String()) || (query.MessageID != strconv.FormatUint(row.MessageID, 10) && query.MessageID != row.MessageUUID.String()) || (query.TraceID != "" && query.TraceID != row.TraceUUID.String()) {
		return nil, agent_run.ErrInvalid
	}
	for attempt := 0; attempt < 5; attempt++ {
		run, err := s.Store.Get(ctx, row.TenantUUID.String(), row.Env, row.UUID.String())
		var archive *agent_run.RunReport
		if errors.Is(err, agent_run.ErrNotFound) && row.ArchivedAt != nil && row.ArchiveKey != "" {
			saved, readErr := agent_run.ReadRunReport(ctx, adminIdentity(row), s.Objects)
			if readErr != nil {
				return nil, readErr
			}
			if saved.Snapshot.Status != row.Status || row.FinishedAt == nil {
				return nil, agent_run.ErrConflict
			}
			run, archive, err = saved.Snapshot, &saved, nil
		}
		if err != nil {
			return nil, err
		}
		var tasks []agent_run.TaskSnapshot
		if archive != nil {
			tasks = archive.Tasks
		} else {
			tasks, err = s.Store.ListTasks(ctx, run, run.PlanRevision)
			if err != nil {
				return nil, err
			}
			if run.PlanRevision > 0 {
				planning, err := s.Store.GetTask(ctx, run, 0, agent_run.PlanningTaskID)
				if err != nil {
					return nil, err
				}
				tasks = append([]agent_run.TaskSnapshot{planning}, tasks...)
			}
		}
		report := &trace.AgentRunReport{ReportScope: "message", Format: "json", TenantUUID: run.TenantUUID, SessionID: strconv.FormatUint(row.SessionID, 10), MessageID: strconv.FormatUint(row.MessageID, 10), RunID: run.RunID, TraceID: run.TraceID, GeneratedBy: "powerx-agent-run-store", GeneratedAt: time.Now().UTC(), RunState: &trace.AgentRunStateSnapshot{Ended: adminTerminal(run.Status), UpdatedAt: run.UpdatedAt}}
		runMap := map[string]any{}
		raw, _ := json.Marshal(run)
		_ = json.Unmarshal(raw, &runMap)
		report.RunState.Run = runMap
		for index, task := range tasks {
			item := trace.AgentTaskStateItem{RunID: run.RunID, TraceID: run.TraceID, SessionID: report.SessionID, MessageID: report.MessageID, TaskID: task.TaskID, Status: task.Status, DependsOn: task.DependsOn, Attempt: task.Attempt, PoolID: task.PoolID, ReasonCode: task.ReasonCode, QueueWaitMS: task.QueueWaitMS, PlanRevision: task.Revision}
			if task.ReasonCode != "" {
				item.Error = map[string]any{"code": task.ReasonCode}
				report.Errors = append(report.Errors, map[string]any{"task_id": task.TaskID, "error_code": task.ReasonCode})
			}
			report.RunState.Tasks = append(report.RunState.Tasks, item)
			node := trace.AgentTraceNodeSnapshot{NodeID: task.TaskID, NodeSeq: index + 1, NodeKind: "task", PhaseStatus: task.Status, ErrorCode: task.ReasonCode, ExecutorPath: "redis-stream-worker", Attributes: map[string]any{"attempt": task.Attempt, "pool_id": task.PoolID, "queue_wait_ms": task.QueueWaitMS, "fence": task.Fence}}
			if !task.StartedAt.IsZero() {
				node.StartedAt = &task.StartedAt
			}
			if !task.CompletedAt.IsZero() {
				node.EndedAt = &task.CompletedAt
			}
			if task.ResultRef != "" {
				node.ArtifactRefs = append(node.ArtifactRefs, task.ResultRef)
				report.ArtifactRefs = append(report.ArtifactRefs, task.ResultRef)
			}
			report.Nodes = append(report.Nodes, node)
		}
		cursor := uint64(0)
		for cursor < run.EventSeq {
			var events []agent_run.Event
			if archive != nil {
				for _, event := range archive.Events {
					if event.Seq > cursor {
						events = append(events, event)
						if len(events) == 1000 {
							break
						}
					}
				}
			} else {
				events, err = s.Store.Events(ctx, run.TenantUUID, run.Env, run.RunID, cursor, 1000)
			}
			if err != nil {
				return nil, err
			}
			if len(events) == 0 {
				return nil, agent_run.ErrConflict
			}
			for _, event := range events {
				if event.Seq > run.EventSeq {
					break
				}
				report.Timeline = append(report.Timeline, trace.AgentTraceEvent{EventID: strconv.FormatUint(event.Seq, 10), TraceID: run.TraceID, RunID: run.RunID, TenantUUID: run.TenantUUID, SessionID: report.SessionID, MessageID: report.MessageID, AgentID: strconv.FormatUint(row.AgentID, 10), NodeID: event.TaskID, NodeKind: "task", Phase: event.Type, Status: event.Status, ErrorCode: event.ReasonCode, CreatedAt: event.CreatedAt, Attributes: map[string]any{"event_seq": event.Seq, "attempt": event.Attempt, "pool_id": event.PoolID, "queue_wait_ms": event.QueueWaitMS}})
				cursor = event.Seq
			}
		}
		backend := "object_storage"
		if archive == nil {
			latest, err := s.Store.Get(ctx, run.TenantUUID, run.Env, run.RunID)
			if err != nil {
				return nil, err
			}
			if latest.Version != run.Version {
				continue
			}
			backend = "redis"
		}
		report.Summary = map[string]any{"status": run.Status, "agent_id": row.AgentID, "node_count": len(tasks), "event_count": run.EventSeq, "error_count": len(report.Errors), "started_at": run.CreatedAt, "updated_at": run.UpdatedAt, "duration_ms": run.UpdatedAt.Sub(run.CreatedAt).Milliseconds(), "state_backend": backend, "plan_revision": run.PlanRevision}
		report.RunState.Summary = report.Summary
		return report, nil
	}
	return nil, fmt.Errorf("run report changed during read; retry")
}
