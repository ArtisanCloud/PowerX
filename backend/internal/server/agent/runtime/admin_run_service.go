package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	chatmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	mediaservice "github.com/ArtisanCloud/PowerX/internal/service/media"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AdminRunService 管理用户聊天 Run 的受理、恢复索引和唯一结果消息。
// 与插件服务会话使用独立所有权记录，复用同一调度协议。
type AdminRunService struct {
	repo              *repo.AdminRunRepository
	Store             *agent_run.RedisStore
	Queue             agent_run.TaskEnqueuer
	Objects           agent_run.ReportObjectStore
	Loader            *adminInputLoader
	Output            *DurableFinalOutputReader
	Env               string
	archiveRecovery   bool
	retentionRecovery bool
	AdmissionGate     func(context.Context) error
	Deadline          time.Duration
}

func NewAdminRunService(db *gorm.DB, store *agent_run.RedisStore, queue agent_run.TaskEnqueuer, objects agent_run.ReportObjectStore, env string, deadline time.Duration) *AdminRunService {
	return &AdminRunService{repo: repo.NewAdminRunRepository(db), Store: store, Queue: queue, Objects: objects, Loader: newAdminInputLoader(db, objects), Output: &DurableFinalOutputReader{Runs: store, Objects: objects}, Env: env, Deadline: deadline}
}
func adminIdentity(row *model.AdminRunAdmission) agent_run.Snapshot {
	return agent_run.Snapshot{TenantUUID: row.TenantUUID.String(), Env: row.Env, RunID: row.UUID.String(), SessionID: row.SessionUUID.String(), MessageID: row.MessageUUID.String(), TraceID: row.TraceUUID.String(), Status: "accepted", DeadlineAt: row.DeadlineAt}
}

func (s *AdminRunService) Admit(ctx context.Context, message string, cfg *dto.ChatConfig, flow, key string) (agent_run.Snapshot, error) {
	if s == nil || s.Store == nil || s.Queue == nil || s.Objects == nil || s.Deadline < time.Minute || len(key) > 128 {
		return agent_run.Snapshot{}, agent_run.ErrInvalid
	}
	tenant, err := uuid.Parse(reqctx.GetTenantUUID(ctx))
	if err != nil {
		return agent_run.Snapshot{}, agent_run.ErrInvalid
	}
	subject, err := uuid.Parse(reqctx.GetUserUUID(ctx))
	if err != nil {
		return agent_run.Snapshot{}, agent_run.ErrInvalid
	}
	sessionID, err := uuid.Parse(strings.TrimSpace(fmt.Sprint(ctx.Value("session_uuid"))))
	if err != nil {
		return agent_run.Snapshot{}, agent_run.ErrInvalid
	}
	messageID, err := uuid.Parse(strings.TrimSpace(fmt.Sprint(ctx.Value("message_uuid"))))
	if err != nil {
		return agent_run.Snapshot{}, agent_run.ErrInvalid
	}
	runID, err := uuid.Parse(strings.TrimSpace(fmt.Sprint(ctx.Value("runtime_run_uuid"))))
	if err != nil {
		return agent_run.Snapshot{}, agent_run.ErrInvalid
	}
	traceID, err := uuid.Parse(reqctx.GetTraceID(ctx))
	if err != nil {
		return agent_run.Snapshot{}, agent_run.ErrInvalid
	}
	snapshot, ok := ResourceSnapshotFromContext(ctx)
	if !ok || snapshot.TenantUUID != tenant.String() {
		return agent_run.Snapshot{}, agent_run.ErrInvalid
	}
	if key == "" {
		key = messageID.String()
	}
	var member *uuid.UUID
	if text := reqctx.GetMemberUUID(ctx); text != "" {
		id, err := uuid.Parse(text)
		if err != nil {
			return agent_run.Snapshot{}, agent_run.ErrInvalid
		}
		member = &id
	}
	actor, _, err := s.repo.Actor(ctx, tenant, subject, member)
	if err != nil {
		return agent_run.Snapshot{}, fmt.Errorf("admin run actor is unavailable")
	}
	// 已受理请求复用冻结输入；背压期间续订不再写入对象或改变原输入。
	var inputRef, checksum string
	previous, readErr := s.repo.ByKey(ctx, tenant, s.Env, sessionID, key)
	if readErr == nil {
		inputRef, checksum = previous.InputRef, previous.InputChecksum
	} else {
		if !errors.Is(readErr, gorm.ErrRecordNotFound) {
			return agent_run.Snapshot{}, readErr
		}
		if s.AdmissionGate != nil {
			if err := s.AdmissionGate(ctx); err != nil {
				return agent_run.Snapshot{}, err
			}
		}
		ref := agent_run.TaskRef{TenantUUID: tenant.String(), Env: s.Env, RunID: runID.String()}
		inputRef, checksum, err = saveAdminRunInput(ctx, s.Objects, ref, message, cfg, flow)
		if err != nil {
			return agent_run.Snapshot{}, err
		}
	}
	var anchor *model.AdminRunAdmission
	err = s.repo.WithSession(ctx, tenant, s.Env, sessionID, func(tx *repo.AdminRunRepository, session *chatmodel.AgentChatSession) error {
		if session.Status != "active" || (session.UserID != 0 && session.UserID != actor.ID) {
			return agent_run.ErrConflict
		}
		existing, err := tx.ByKey(ctx, tenant, s.Env, sessionID, key)
		if err == nil {
			if existing.MessageUUID != messageID || existing.SubjectUUID != subject {
				return agent_run.ErrConflict
			}
			anchor = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if s.AdmissionGate != nil {
			if err := s.AdmissionGate(ctx); err != nil {
				return err
			}
		}
		if active, err := tx.Active(ctx, tenant, s.Env, sessionID); err != nil {
			return err
		} else if active {
			return agent_run.ErrConflict
		}
		msg, err := tx.Message(ctx, tenant, s.Env, messageID)
		if err != nil {
			return err
		}
		if msg.SessionID != session.ID || msg.AgentID != session.AgentID || msg.Role != "user" || strings.TrimSpace(msg.Content) != strings.TrimSpace(message) {
			return agent_run.ErrInvalid
		}
		deadline := time.Now().Add(s.Deadline)
		// 受理不得延长冻结授权的有效期；前置规划耗时计入可恢复窗口。
		if snapshot.ExpiresAt.IsZero() || !snapshot.ExpiresAt.After(time.Now()) {
			return fmt.Errorf("runtime snapshot expired before admission")
		}
		if snapshot.ExpiresAt.Before(deadline) {
			deadline = snapshot.ExpiresAt
		}
		anchor = &model.AdminRunAdmission{TenantUUID: tenant, Env: s.Env, SessionUUID: sessionID, MessageUUID: messageID, AgentUUID: snapshot.AgentUUID, SubjectUUID: subject, MemberUUID: member, SessionID: session.ID, MessageID: msg.ID, AgentID: session.AgentID, IdempotencyKey: key, InputRef: inputRef, InputChecksum: checksum, TraceUUID: traceID, SnapshotUUID: snapshot.SnapshotUUID, Status: "accepted", AdmissionState: "pending_create", DeadlineAt: deadline.Truncate(time.Microsecond)}
		anchor.UUID = runID
		return tx.Insert(ctx, anchor)
	})
	if err != nil {
		return agent_run.Snapshot{}, err
	}
	identity := adminIdentity(anchor)
	if anchor.AdmissionState == "pending_create" {
		if _, err = s.Store.Create(ctx, identity); err != nil {
			return agent_run.Snapshot{}, err
		}
		if err = s.repo.MarkAdmitted(ctx, anchor); err != nil {
			latest, readErr := s.repo.Get(ctx, tenant, s.Env, anchor.UUID)
			if readErr != nil || latest.AdmissionState != "admitted" {
				return agent_run.Snapshot{}, err
			}
		}
	} else if anchor.AdmissionState != "admitted" {
		return agent_run.Snapshot{}, agent_run.ErrConflict
	}
	current, err := s.Store.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
	if errors.Is(err, agent_run.ErrNotFound) && anchor.ArchivedAt != nil {
		restored, _, readErr := s.readRunForSubscription(ctx, anchor)
		return restored, readErr
	}
	if err != nil {
		return agent_run.Snapshot{}, err
	}
	if current.SessionID != identity.SessionID || current.MessageID != identity.MessageID || current.TraceID != identity.TraceID || !current.DeadlineAt.Equal(identity.DeadlineAt) {
		return agent_run.Snapshot{}, agent_run.ErrConflict
	}
	return s.Store.RecoverRun(ctx, identity, s.Queue)
}
func (s *AdminRunService) ListUnfinishedRuns(ctx context.Context, after uint64, limit int) ([]agent_run.LocatedRun, error) {
	rows, err := s.repo.ListUnfinished(ctx, s.Env, after, limit, s.archiveRecovery, s.retentionRecovery)
	if err != nil {
		return nil, err
	}
	result := make([]agent_run.LocatedRun, 0, len(rows))
	for i := range rows {
		if rows[i].AdmissionState == "pending_create" {
			// 创建与提升均幂等。仅 pending_create 可补建；已受理的 Redis 丢失绝不重建。
			row := &rows[i]
			err := s.repo.WithSession(ctx, row.TenantUUID, row.Env, row.SessionUUID, func(tx *repo.AdminRunRepository, _ *chatmodel.AgentChatSession) error {
				latest, err := tx.Get(ctx, row.TenantUUID, row.Env, row.UUID)
				if err != nil {
					return err
				}
				if latest.AdmissionState != "pending_create" {
					return nil
				}
				if _, err := s.Store.RecoverAdmission(ctx, adminIdentity(latest)); err != nil {
					return err
				}
				return tx.MarkAdmitted(ctx, latest)
			})
			if err != nil {
				return nil, err
			}
		}
		result = append(result, agent_run.LocatedRun{Cursor: rows[i].ID, Identity: adminIdentity(&rows[i])})
	}
	return result, nil
}
func (s *AdminRunService) FinalizeRun(ctx context.Context, identity agent_run.Snapshot) error {
	tenant, err := uuid.Parse(identity.TenantUUID)
	if err != nil {
		return agent_run.ErrInvalid
	}
	id, err := uuid.Parse(identity.RunID)
	if err != nil {
		return agent_run.ErrInvalid
	}
	row, err := s.repo.Get(ctx, tenant, identity.Env, id)
	if err != nil {
		return err
	}
	run, err := s.Store.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
	if err != nil {
		return err
	}
	if !adminTerminal(run.Status) || run.SessionID != row.SessionUUID.String() || run.MessageID != row.MessageUUID.String() || run.TraceID != row.TraceUUID.String() || !run.DeadlineAt.Equal(row.DeadlineAt) {
		return agent_run.ErrConflict
	}
	if row.FinishedAt != nil {
		return nil
	}
	var content string
	meta := datatypes.JSONMap{"run_id": run.RunID, "trace_id": run.TraceID, "session_id": strconv.FormatUint(row.SessionID, 10), "message_id": strconv.FormatUint(row.MessageID, 10), "durable_run": true, "status": run.Status}
	tasks, err := s.Store.ListTasks(ctx, run, run.PlanRevision)
	if err != nil {
		return err
	}
	meta["run_state"] = map[string]any{"run_id": run.RunID, "trace_id": run.TraceID, "tasks": tasks, "ended": true, "status": run.Status}
	if run.Status == "completed" {
		output, err := s.Output.ReadFinalOutput(ctx, run)
		if err != nil {
			return err
		}
		content = output.Content
		if len(output.ResponseEnvelope) > 0 {
			var envelope map[string]any
			if err := json.Unmarshal(output.ResponseEnvelope, &envelope); err != nil {
				return err
			}
			meta["response_envelope"] = envelope
		}
	} else {
		content = "本轮执行未完成，请查看运行追踪。"
	}
	return s.repo.WithSession(ctx, tenant, row.Env, row.SessionUUID, func(tx *repo.AdminRunRepository, session *chatmodel.AgentChatSession) error {
		current, err := tx.Get(ctx, tenant, row.Env, id)
		if err != nil {
			return err
		}
		if current.FinishedAt != nil {
			return nil
		}
		tenantText := tenant.String()
		message := &chatmodel.AgentChatMessage{Env: row.Env, TenantUUID: &tenantText, SessionID: row.SessionID, AgentID: row.AgentID, Role: "assistant", Content: content, ContentType: "text", Format: "text", SizeBytes: len(content), IsError: run.Status != "completed", Meta: meta}
		message.UUID = uuid.NewSHA1(row.UUID, []byte("assistant-result-v1"))
		return tx.Finish(ctx, current, session, message, run.UpdatedAt, run.Status)
	})
}
func adminTerminal(status string) bool {
	switch status {
	case "completed", "partial", "needs_input", "blocked", "failed", "cancelled":
		return true
	}
	return false
}

func (s *AdminRunService) ConfigureMedia(service *mediaservice.MediaService) {
	s.Loader.media = service
}
