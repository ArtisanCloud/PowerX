package agent_session

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	"strings"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const InvokeCapability = "com.corex.agent.invoke"
const ExecutionTTL = 10 * time.Minute

// Executor is the explicit Core runtime boundary. A missing executor is an
// error before an invocation is created, never a successful no-op execution.
type Executor interface {
	Execute(context.Context, Session, Message) (string, error)
}

func NewServiceWithExecutor(db *gorm.DB, executor Executor) *Service {
	s := NewService(db)
	s.executor = executor
	return s
}

type Invocation struct {
	InvocationUUID   uuid.UUID       `json:"invocation_uuid"`
	SessionUUID      uuid.UUID       `json:"session_uuid"`
	MessageUUID      uuid.UUID       `json:"message_uuid"`
	Status           string          `json:"status"`
	ReasonCode       string          `json:"reason_code"`
	Output           string          `json:"output"`
	TraceUUID        uuid.UUID       `json:"trace_uuid"`
	CreatedAt        time.Time       `json:"created_at"`
	DeadlineAt       time.Time       `json:"deadline_at"`
	FinishedAt       *time.Time      `json:"finished_at"`
	ResponseEnvelope json.RawMessage `json:"response_envelope,omitempty"`
}

func invocationDTO(run *m.ServiceInvocation) Invocation {
	return Invocation{run.UUID, run.SessionUUID, run.MessageUUID, run.Status, run.ReasonCode, run.Output, run.TraceUUID, run.CreatedAt, run.DeadlineAt, run.FinishedAt, json.RawMessage(run.ResponseEnvelope)}
}

func (s *Service) Invoke(ctx context.Context, id, messageID uuid.UUID, key string) (Invocation, error) {
	owner, err := s.owner(ctx, SessionCapability)
	if err != nil {
		return Invocation{}, err
	}
	if _, err = s.owner(ctx, InvokeCapability); err != nil {
		return Invocation{}, err
	}
	if s.executor == nil && s.durableRuns == nil {
		return Invocation{}, ErrDependency
	}
	if id == uuid.Nil || messageID == uuid.Nil || key == "" || len(key) > 128 || strings.TrimSpace(key) != key {
		return Invocation{}, ErrInvalid
	}
	var run *m.ServiceInvocation
	var sessionDTOValue Session
	var messageDTOValue Message
	created := false
	memory := &ExecutionMemory{}
	err = s.repo.WithSession(ctx, owner, id, func(tx *repo.ServiceSessionRepository, session *m.ServiceSession) error {
		if session.Status == "deleted" {
			return ErrNotFound
		}
		if err := agentAllowed(ctx, tx, owner, session.AgentUUID); err != nil {
			return err
		}
		if err := tx.ExpireInvocations(ctx, session, s.now()); err != nil {
			return err
		}
		previous, err := tx.InvocationByKey(ctx, session, key)
		if err == nil {
			if previous.MessageUUID != messageID {
				return ErrConflict
			}
			if !s.now().Before(previous.IdempotencyExpiresAt) {
				return ErrExpired
			}
			if (s.durableRuns != nil) != (previous.AdmissionState != "") {
				return ErrConflict
			}
			run = previous
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if session.Status != "active" {
			return ErrConflict
		}
		active, err := tx.ActiveInvocations(ctx, session)
		if err != nil {
			return err
		}
		if active > 0 {
			return ErrConflict
		}
		message, err := tx.Message(ctx, session, messageID)
		if err != nil {
			return err
		}
		if message.Role != "user" {
			return ErrInvalid
		}
		history, err := tx.ExecutionHistory(ctx, session, message.Sequence, MaxExecutionHistoryMessages+1)
		if err != nil {
			return err
		}
		if len(history) > MaxExecutionHistoryMessages {
			return ErrConflict
		}
		bytes := 0
		for _, row := range history {
			if row.Role != "user" && row.Role != "assistant" {
				return ErrDependency
			}
			bytes += len(row.Content) + len(row.ResponseEnvelope)
			memory.History = append(memory.History, messageDTO(&row))
		}
		if bytes > MaxExecutionHistoryBytes {
			return ErrConflict
		}
		if len(session.PendingTask) > 0 && string(session.PendingTask) != "null" {
			if session.PendingTaskExpiresAt == nil || !s.now().Before(*session.PendingTaskExpiresAt) {
				return ErrContextExpired
			}
			if err := json.Unmarshal(session.PendingTask, &memory.Pending); err != nil {
				return ErrDependency
			}
			if memory.Pending["status"] != "awaiting_params" {
				return ErrDependency
			}
			ref, ok := memory.Pending["node_ref"].(string)
			if !ok || strings.TrimSpace(ref) == "" {
				return ErrDependency
			}
		}
		status, deadline := "running", s.now().Add(ExecutionTTL)
		admissionState, runEnv := "", ""
		if s.durableRuns != nil {
			// PostgreSQL stores timestamps to microseconds; Redis must receive the same canonical deadline.
			status, deadline = "accepted", s.now().Add(s.runDeadline).Truncate(time.Microsecond)
			admissionState, runEnv = "pending_create", s.runEnv
		}
		run = &m.ServiceInvocation{MessageUUID: messageID, Status: status, RunEnv: runEnv, AdmissionState: admissionState,
			IdempotencyKey: key, RequestHash: messageID.String(), IdempotencyExpiresAt: s.now().Add(IdempotencyTTL), DeadlineAt: deadline, TraceUUID: uuid.New()}
		if err := tx.InsertInvocation(ctx, owner, session, run); err != nil {
			return err
		}
		sessionDTOValue = sessionDTO(session)
		messageDTOValue = messageDTO(message)
		created = true
		return nil
	})
	if err != nil {
		return Invocation{}, translate(err)
	}
	if s.durableRuns != nil {
		if err := s.admitDurable(ctx, owner, run); err != nil {
			return Invocation{}, err
		}
		if err := s.submitPlanning(ctx, owner, run); err != nil {
			return Invocation{}, err
		}
	} else if created {
		go s.execute(context.WithValue(context.WithoutCancel(ctx), executionMemoryKey{}, memory), owner, run, sessionDTOValue, messageDTOValue)
	}
	return invocationDTO(run), nil
}

func (s *Service) execute(ctx context.Context, owner repo.SessionOwner, run *m.ServiceInvocation, session Session, message Message) {
	ctx = reqctx.WithTraceID(ctx, run.TraceUUID.String())
	ctx = context.WithValue(ctx, "run_id", run.UUID.String())
	ctx, cancel := context.WithDeadline(ctx, run.DeadlineAt)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				current, err := s.repo.Invocation(ctx, owner, run.SessionUUID, run.UUID)
				if err != nil || current.CancelRequestedAt != nil {
					cancel()
					return
				}
			}
		}
	}()
	var output string
	var err error
	func() {
		defer func() {
			if recover() != nil {
				err = ErrDependency
			}
		}()
		output, err = s.executor.Execute(ctx, session, message)
	}()
	status, reason := "succeeded", ""
	if err != nil || ctx.Err() != nil {
		status = "failed"
		reason = ErrDependency.Error()
		output = ""
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	// Failed persistence is observable to the reader via the stored execution
	// deadline. It must never be retried as a new Agent execution.
	err = s.repo.WithSession(finishCtx, owner, run.SessionUUID, func(tx *repo.ServiceSessionRepository, session *m.ServiceSession) error {
		current, err := tx.Invocation(finishCtx, owner, run.SessionUUID, run.UUID)
		if err != nil {
			return err
		}
		if current.Status != "running" && current.Status != "cancelling" {
			return nil
		}
		if current.CancelRequestedAt != nil {
			status = "cancelled"
			reason = "AGENT_SESSION_CANCELLED"
			output = ""
		}
		if status == "succeeded" {
			memory := ExecutionMemoryFromContext(ctx)
			if memory == nil {
				return ErrDependency
			}
			pending, err := memory.pendingJSON()
			if err != nil {
				return err
			}
			session.PendingTask = pending
			session.PendingTaskExpiresAt = nil
			if string(pending) != "null" {
				expires := s.now().Add(72 * time.Hour)
				session.PendingTaskExpiresAt = &expires
			}
			session.Revision++
			message := &m.ServiceMessage{Role: "assistant", Content: output, Sequence: session.Revision, IdempotencyKey: "invoke:" + run.UUID.String(), RequestHash: run.UUID.String(), IdempotencyExpiresAt: run.IdempotencyExpiresAt}
			if err := tx.InsertMessage(finishCtx, session, message); err != nil {
				return err
			}
			if err := tx.SaveSession(finishCtx, owner, session); err != nil {
				return err
			}
		}
		return tx.FinishInvocation(finishCtx, owner, run, status, reason, output)
	})
	if err != nil {
		logger.ErrorF(finishCtx, "agent_session.finish_failed invocation_uuid=%s tenant_uuid=%s error=%v", run.UUID, owner.TenantUUID, err)
	}
}

func (s *Service) GetInvocation(ctx context.Context, sessionID, id uuid.UUID) (Invocation, error) {
	owner, err := s.owner(ctx, SessionCapability)
	if err != nil {
		return Invocation{}, err
	}
	if sessionID == uuid.Nil || id == uuid.Nil {
		return Invocation{}, ErrInvalid
	}
	if _, err = s.repo.GetSession(ctx, owner, sessionID); err != nil {
		return Invocation{}, translate(err)
	}
	run, err := s.repo.Invocation(ctx, owner, sessionID, id)
	if err != nil {
		return Invocation{}, translate(err)
	}
	if run.AdmissionState != "" {
		if s.durableRuns == nil || run.AdmissionState != "admitted" || run.RunEnv != s.runEnv {
			return Invocation{}, ErrDependency
		}
		current, err := s.durableRuns.Get(ctx, owner.TenantUUID.String(), run.RunEnv, run.UUID.String())
		if err != nil || current.SessionID != sessionID.String() || current.MessageID != run.MessageUUID.String() ||
			current.TraceID != run.TraceUUID.String() || !current.DeadlineAt.Equal(run.DeadlineAt) {
			return Invocation{}, ErrDependency
		}
		// A terminal result is only exposed after the single assistant message is committed.
		if durableTerminalStatus(current.Status) && run.FinishedAt == nil && s.finalOutput != nil {
			if err := s.FinalizeRun(ctx, current); err != nil {
				return Invocation{}, ErrDependency
			}
			run, err = s.repo.Invocation(ctx, owner, sessionID, id)
			if err != nil {
				return Invocation{}, translate(err)
			}
		}
		result := invocationDTO(run)
		result.Status = current.Status
		if durableTerminalStatus(current.Status) {
			finished := current.UpdatedAt
			result.FinishedAt = &finished
		}
		return result, nil
	}
	if (run.Status == "running" || run.Status == "cancelling") && !s.now().Before(run.DeadlineAt) {
		err = s.repo.WithSession(ctx, owner, sessionID, func(tx *repo.ServiceSessionRepository, _ *m.ServiceSession) error {
			current, err := tx.Invocation(ctx, owner, sessionID, id)
			if err != nil {
				return err
			}
			if (current.Status == "running" || current.Status == "cancelling") && !s.now().Before(current.DeadlineAt) {
				return tx.FinishInvocation(ctx, owner, current, "failed", "AGENT_SESSION_EXECUTION_EXPIRED", "")
			}
			return nil
		})
		if err != nil {
			return Invocation{}, translate(err)
		}
		run, err = s.repo.Invocation(ctx, owner, sessionID, id)
		if err != nil {
			return Invocation{}, translate(err)
		}
	}
	return invocationDTO(run), nil
}
func (s *Service) Cancel(ctx context.Context, sessionID, id uuid.UUID) (Invocation, error) {
	owner, err := s.owner(ctx, SessionCapability)
	if err != nil {
		return Invocation{}, err
	}
	if _, err = s.GetInvocation(ctx, sessionID, id); err != nil {
		return Invocation{}, err
	}
	if s.durableRuns != nil {
		canceller, ok := s.durableRuns.(interface {
			Cancel(context.Context, agent_run.Snapshot) (agent_run.Snapshot, error)
		})
		if !ok {
			return Invocation{}, ErrDependency
		}
		_, err := canceller.Cancel(ctx, agent_run.Snapshot{TenantUUID: owner.TenantUUID.String(), Env: s.runEnv, RunID: id.String()})
		if err != nil {
			return Invocation{}, ErrDependency
		}
		return s.GetInvocation(ctx, sessionID, id)
	}
	err = s.repo.WithSession(ctx, owner, sessionID, func(tx *repo.ServiceSessionRepository, _ *m.ServiceSession) error {
		return tx.CancelInvocation(ctx, owner, sessionID, id)
	})
	if err != nil {
		return Invocation{}, translate(err)
	}
	return s.GetInvocation(ctx, sessionID, id)
}
