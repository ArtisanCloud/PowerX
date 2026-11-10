package runtimescheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const TaskDispatchContract = "scheduler.taskqueue.v1"

type TaskDispatch struct {
	Contract     string          `json:"contract"`
	SubscriberID string          `json:"subscriber_id"`
	Payload      json.RawMessage `json:"payload"`
}

type TaskPublisher interface {
	PublishScheduledTask(context.Context, *models.SchedulerJobRun, TaskDispatch) error
}

func taskDispatchDeclaration(payload map[string]any) (*TaskDispatch, error) {
	raw, exists := payload["task_dispatch"]
	if !exists {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, errors.New("SCHEDULER_TASK_DISPATCH_INVALID")
	}
	var dispatch TaskDispatch
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&dispatch) != nil || decoder.Decode(new(any)) != io.EOF || len(dispatch.Payload) > 0 || dispatch.SubscriberID != strings.TrimSpace(dispatch.SubscriberID) || dispatch.Contract != TaskDispatchContract || strings.TrimSpace(dispatch.SubscriberID) == "" || len(dispatch.SubscriberID) > 128 || strings.TrimSpace(stringValue(payload["business_action"])) == "" {
		return nil, errors.New("SCHEDULER_TASK_DISPATCH_INVALID")
	}
	return &dispatch, nil
}

func stringValue(v any) string { s, _ := v.(string); return s }

func (s *Service) prepareTaskDispatch(job *models.SchedulerJob, run *models.SchedulerJobRun, firedAt time.Time) error {
	var payload map[string]any
	if json.Unmarshal(job.PayloadJSON, &payload) != nil {
		return errors.New("SCHEDULER_TASK_DISPATCH_INVALID")
	}
	dispatch, err := taskDispatchDeclaration(payload)
	if err != nil || dispatch == nil {
		return err
	}
	if job.OwnerType != models.OwnerTypePlugin {
		return errors.New("SCHEDULER_TASK_PLUGIN_OWNER_REQUIRED")
	}
	if run.UUID == uuid.Nil {
		run.UUID = uuid.New()
	}
	run.EventID = "scheduler." + run.UUID.String()
	dispatch.Payload, err = json.Marshal(s.buildTriggerPayload(job, run, run.TriggerSource, firedAt))
	if err != nil {
		return err
	}
	raw, err := json.Marshal(dispatch)
	if err != nil {
		return err
	}
	run.DispatchPayloadJSON = datatypes.JSON(raw)
	run.Status = models.RunStatusPendingDispatch
	now := firedAt.UTC()
	run.NextDispatchAt = &now
	return nil
}

func (s *Service) publishRun(ctx context.Context, job *models.SchedulerJob, run *models.SchedulerJobRun, at time.Time) error {
	if len(run.DispatchPayloadJSON) > 0 {
		return s.dispatchPendingRun(ctx, run.UUID, at)
	}
	if s.eventBu != nil {
		s.eventBu.Publish(models.TopicSchedulerTriggeredV1, s.buildTriggerPayload(job, run, run.TriggerSource, at), ctx)
	}
	return nil
}

// Persisted trigger payloads are retried independently of future job revisions.
// Queue message identity is the run event ID, never the recurring job's key.
func (s *Service) dispatchPendingRun(ctx context.Context, id uuid.UUID, now time.Time) error {
	var run models.SchedulerJobRun
	if err := s.db.WithContext(ctx).Where("uuid = ?", id).First(&run).Error; err != nil {
		return err
	}
	return s.withJobFence(ctx, run.JobUUID.String(), func(locked *Service) error { return locked.dispatchPendingRunLocked(ctx, id, now) })
}
func (s *Service) dispatchPendingRunLocked(ctx context.Context, id uuid.UUID, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run models.SchedulerJobRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ?", id).First(&run).Error; err != nil {
			return err
		}
		if run.Status != models.RunStatusPendingDispatch || run.NextDispatchAt == nil || run.NextDispatchAt.After(now) {
			return nil
		}
		var job models.SchedulerJob
		if err := tx.Unscoped().Where("uuid = ? AND tenant_uuid = ? AND owner_type = ? AND owner_id = ?", run.JobUUID, run.TenantUUID, run.OwnerType, run.OwnerID).First(&job).Error; err != nil {
			return err
		}
		if job.DeletedAt.Valid || job.Status == models.JobStatusPaused || job.Status == models.JobStatusDeleted {
			return tx.Model(&run).Updates(map[string]any{"status": models.RunStatusSkipped, "error_code": "SCHEDULER_JOB_INACTIVE", "next_dispatch_at": nil}).Error
		}
		var dispatch TaskDispatch
		if json.Unmarshal(run.DispatchPayloadJSON, &dispatch) != nil || dispatch.Contract != TaskDispatchContract {
			return tx.Model(&run).Updates(map[string]any{"status": models.RunStatusFailed, "error_code": "SCHEDULER_TASK_DISPATCH_INVALID", "next_dispatch_at": nil}).Error
		}
		run.DispatchAttempts++
		var err error
		if s.taskPublisher == nil {
			err = errors.New("SCHEDULER_TASK_QUEUE_UNAVAILABLE")
		} else {
			err = s.taskPublisher.PublishScheduledTask(ctx, &run, dispatch)
		}
		updates := map[string]any{"dispatch_attempts": run.DispatchAttempts, "next_dispatch_at": nil, "error_code": "", "error_message": ""}
		if err == nil {
			updates["status"] = models.RunStatusQueued
		} else {
			updates["error_code"] = "SCHEDULER_TASK_DISPATCH_FAILED"
			updates["error_message"] = "SCHEDULER_TASK_DISPATCH_FAILED"
			if run.DispatchAttempts >= 5 {
				updates["status"] = models.RunStatusFailed
			} else {
				retry := now.Add(time.Duration(1<<uint(run.DispatchAttempts-1)) * 5 * time.Second)
				updates["next_dispatch_at"] = &retry
			}
		}
		if updateErr := tx.Model(&run).Updates(updates).Error; updateErr != nil {
			return updateErr
		}
		return nil
	})
}

func (s *Service) dispatchPendingTasks(ctx context.Context, now time.Time, limit int) error {
	var runs []models.SchedulerJobRun
	if err := s.db.WithContext(ctx).Where("status = ? AND next_dispatch_at <= ?", models.RunStatusPendingDispatch, now).Order("next_dispatch_at ASC").Limit(limit).Find(&runs).Error; err != nil {
		return err
	}
	for _, run := range runs {
		if err := s.dispatchPendingRun(ctx, run.UUID, now); err != nil {
			return err
		}
	}
	return nil
}
