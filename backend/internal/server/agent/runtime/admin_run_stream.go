package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
)

type RunEventEmitter func(uint64, string, any) error

func (s *AdminRunService) AuthorizedRun(ctx context.Context, id string) (*model.AdminRunAdmission, error) {
	tenant, err := uuid.Parse(reqctx.GetTenantUUID(ctx))
	if err != nil {
		return nil, agent_run.ErrInvalid
	}
	runID, err := uuid.Parse(id)
	if err != nil {
		return nil, agent_run.ErrInvalid
	}
	row, err := s.repo.Get(ctx, tenant, s.Env, runID)
	if err != nil {
		return nil, err
	}
	if row.SubjectUUID.String() != reqctx.GetUserUUID(ctx) {
		return nil, fmt.Errorf("admin run owner mismatch")
	}
	if _, _, err := s.repo.Actor(ctx, tenant, row.SubjectUUID, row.MemberUUID); err != nil {
		return nil, err
	}
	return row, nil
}

// ValidateSubscription 在写出 SSE 响应头前校验所有权和游标。
func (s *AdminRunService) ValidateSubscription(ctx context.Context, id string, cursor uint64) error {
	row, err := s.AuthorizedRun(ctx, id)
	if err != nil {
		return err
	}
	run, err := s.Store.Get(ctx, row.TenantUUID.String(), row.Env, id)
	if err != nil {
		return err
	}
	if cursor > run.EventSeq {
		return agent_run.ErrInvalid
	}
	return nil
}

// Subscribe 仅订阅已受理 Run；断线不取消执行，也不创建另一条用户或助手消息。
func (s *AdminRunService) Subscribe(ctx context.Context, id string, cursor uint64, emit RunEventEmitter) error {
	row, err := s.AuthorizedRun(ctx, id)
	if err != nil {
		return err
	}
	identity := adminIdentity(row)
	if emit == nil {
		return agent_run.ErrInvalid
	}
	if err := emit(0, dto.EventMeta, map[string]any{"run_id": id, "trace_id": row.TraceUUID.String(), "session_id": strconv.FormatUint(row.SessionID, 10), "session_uuid": row.SessionUUID.String(), "message_id": strconv.FormatUint(row.MessageID, 10), "durable_run": true}); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastAuth := time.Now()
	for {
		run, err := s.Store.Get(ctx, identity.TenantUUID, identity.Env, id)
		if err != nil {
			return err
		}
		if cursor > run.EventSeq {
			return agent_run.ErrInvalid
		}
		events, err := s.Store.Events(ctx, identity.TenantUUID, identity.Env, id, cursor, 100)
		if err != nil && !errors.Is(err, agent_run.ErrEventCursorExpired) {
			return err
		}
		// 每次重连均发送当前任务快照，避免游标裁剪或断线落在一组快照中间。
		tasks, err := s.Store.ListTasks(ctx, run, run.PlanRevision)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			cursor = run.EventSeq
		}
		for _, event := range events {
			eventType := event.Type
			if event.TaskID != "" {
				eventType = dto.EventAgentRunTaskStatus
			}
			payload := dto.AgentRunEvent{RunID: id, SessionID: strconv.FormatUint(row.SessionID, 10), MessageID: strconv.FormatUint(row.MessageID, 10), TraceID: row.TraceUUID.String(), EventSeq: event.Seq, Event: eventType, Payload: event}
			if err := emit(event.Seq, eventType, payload); err != nil {
				return err
			}
			cursor = event.Seq
		}
		for _, task := range tasks {
			payload := dto.AgentTaskState{RunID: id, SessionID: strconv.FormatUint(row.SessionID, 10), MessageID: strconv.FormatUint(row.MessageID, 10), TraceID: row.TraceUUID.String(), TaskID: task.TaskID, Status: task.Status, PlanRevision: task.Revision, Attempt: task.Attempt, PoolID: task.PoolID, ReasonCode: task.ReasonCode, DependsOn: task.DependsOn, QueueReason: task.QueueReason}
			if err := emit(0, dto.EventAgentRunTaskStatus, dto.AgentRunEvent{RunID: id, SessionID: payload.SessionID, MessageID: payload.MessageID, TraceID: payload.TraceID, Event: dto.EventAgentRunTaskStatus, Payload: payload}); err != nil {
				return err
			}
		}
		if adminTerminal(run.Status) && cursor >= run.EventSeq {
			if err := s.FinalizeRun(ctx, run); err != nil {
				return err
			}
			current, err := s.repo.Get(ctx, row.TenantUUID, row.Env, row.UUID)
			if err != nil {
				return err
			}
			if current.AssistantMessageUUID == nil {
				return fmt.Errorf("admin result message unavailable")
			}
			message, err := s.repo.Message(ctx, row.TenantUUID, row.Env, *current.AssistantMessageUUID)
			if err != nil {
				return err
			}
			// 完成消息已入库；流只投影同一条消息，不经过旧 HistorySink 再次保存。
			data := map[string]any{"content": message.Content}
			if envelope, ok := message.Meta["response_envelope"]; ok {
				data["response_envelope"] = envelope
			}
			if err := emit(0, dto.EventMeta, map[string]any{"assistant_message_id": message.ID, "assistant_message_uuid": message.UUID.String(), "run_id": id}); err != nil {
				return err
			}
			if err := emit(0, dto.EventFinal, map[string]any{"success": run.Status == "completed", "data": data, "metadata": message.Meta}); err != nil {
				return err
			}
			if err := emit(0, dto.EventAgentRunEnded, dto.AgentRunEvent{RunID: id, Event: dto.EventAgentRunEnded, Payload: map[string]any{"status": run.Status, "success": run.Status == "completed"}}); err != nil {
				return err
			}
			return emit(0, dto.EventEnd, map[string]any{"success": run.Status == "completed"})
		}
		if cursor < run.EventSeq {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if time.Since(lastAuth) >= 10*time.Second {
			if _, err = s.AuthorizedRun(ctx, id); err != nil {
				return err
			}
			lastAuth = time.Now()
		}
	}
}

// 用于同步入口，结果仍从同一已提交的助手消息读取。
func (s *AdminRunService) WaitResult(ctx context.Context, id string) (map[string]any, error) {
	var result map[string]any
	err := s.Subscribe(ctx, id, 0, func(_ uint64, event string, payload any) error {
		if event == dto.EventFinal {
			raw, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			return json.Unmarshal(raw, &result)
		}
		return nil
	})
	return result, err
}
