package agent_run

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	"github.com/redis/go-redis/v9"
)

const maxRunReportBytes = 16 << 20

type ReportObjectStore interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
}

type MediaReportObjects struct {
	Manager *manager.MediaManager
	Bucket  string
}

func (m MediaReportObjects) Put(ctx context.Context, key string, body []byte) error {
	if m.Manager == nil || strings.TrimSpace(m.Bucket) == "" || len(body) == 0 || len(body) > maxRunReportBytes {
		return ErrInvalid
	}
	_, err := m.Manager.Put(ctx, "s3", driver.PutObjectInput{
		Bucket: m.Bucket, ObjectKey: key, Body: bytes.NewReader(body),
		Size: int64(len(body)), ContentType: "application/json", Overwrite: true,
	})
	return err
}

func (m MediaReportObjects) Get(ctx context.Context, key string) ([]byte, error) {
	if m.Manager == nil || strings.TrimSpace(m.Bucket) == "" {
		return nil, ErrInvalid
	}
	result, err := m.Manager.Get(ctx, "s3", driver.GetObjectInput{Bucket: m.Bucket, ObjectKey: key})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()
	body, err := io.ReadAll(io.LimitReader(result.Body, maxRunReportBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxRunReportBytes {
		return nil, fmt.Errorf("Agent Run report exceeds archive limit")
	}
	return body, nil
}

type RunReport struct {
	SchemaVersion int            `json:"schema_version"`
	Snapshot      Snapshot       `json:"snapshot"`
	Plan          Plan           `json:"plan"`
	Tasks         []TaskSnapshot `json:"tasks"`
	Events        []Event        `json:"events"`
}

type archivedEnvelope struct {
	Checksum string    `json:"checksum"`
	Report   RunReport `json:"report"`
}

// ArchiveRun writes and reads back the report before recording its locator.
// Object storage failure keeps the complete terminal history in Redis.
func (s *RedisStore) ArchiveRun(ctx context.Context, identity Snapshot, objects ReportObjectStore) (string, error) {
	if s == nil || s.client == nil || objects == nil || !validScope(identity) {
		return "", ErrInvalid
	}
	run, err := s.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
	if err != nil {
		return "", err
	}
	if !terminal(run.Status) || !sameReportIdentity(run, identity) {
		return "", ErrConflict
	}
	key := reportObjectKey(identity)
	if run.ArchiveKey != "" {
		if run.ArchiveKey != key {
			return "", ErrConflict
		}
		_, err := ReadRunReport(ctx, identity, objects)
		return key, err
	}
	report := RunReport{SchemaVersion: 1, Snapshot: run}
	if run.PlanRevision > 0 {
		planKey, _, _ := schedulingKeys(identity)
		raw, err := s.client.HGet(ctx, planKey, fmt.Sprintf("%d", run.PlanRevision)).Bytes()
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(raw, &report.Plan); err != nil {
			return "", err
		}
		for _, definition := range report.Plan.Tasks {
			task, err := s.GetTask(ctx, identity, run.PlanRevision, definition.TaskID)
			if err != nil {
				return "", err
			}
			report.Tasks = append(report.Tasks, task)
		}
	}
	var after uint64
	for after < run.EventSeq {
		events, err := s.Events(ctx, identity.TenantUUID, identity.Env, identity.RunID, after, 1000)
		if err != nil {
			return "", err
		}
		if len(events) == 0 {
			return "", ErrEventCursorExpired
		}
		report.Events = append(report.Events, events...)
		after = events[len(events)-1].Seq
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	envelope, err := json.Marshal(archivedEnvelope{Checksum: hex.EncodeToString(sum[:]), Report: report})
	if err != nil || len(envelope) > maxRunReportBytes {
		return "", ErrInvalid
	}
	if err := objects.Put(ctx, key, envelope); err != nil {
		return "", err
	}
	if _, err := ReadRunReport(ctx, identity, objects); err != nil {
		return "", err
	}
	stateKey, _ := runKeys(identity)
	for attempt := 0; attempt < 5; attempt++ {
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.HGetAll(ctx, stateKey).Result()
			if err != nil {
				return err
			}
			current, err := decodeSnapshot(values)
			if err != nil {
				return err
			}
			if !terminal(current.Status) || current.EventSeq != run.EventSeq || current.Version != run.Version {
				return ErrConflict
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, stateKey, "archive_key", key, "archived_at", s.clock().UTC().Format(time.RFC3339Nano))
				return nil
			})
			return err
		}, stateKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return key, err
	}
	return "", ErrConflict
}

func ReadRunReport(ctx context.Context, identity Snapshot, objects ReportObjectStore) (RunReport, error) {
	if objects == nil || !validScope(identity) {
		return RunReport{}, ErrInvalid
	}
	raw, err := objects.Get(ctx, reportObjectKey(identity))
	if err != nil {
		return RunReport{}, err
	}
	if len(raw) == 0 || len(raw) > maxRunReportBytes {
		return RunReport{}, ErrInvalid
	}
	var envelope archivedEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return RunReport{}, err
	}
	payload, err := json.Marshal(envelope.Report)
	if err != nil {
		return RunReport{}, err
	}
	sum := sha256.Sum256(payload)
	if envelope.Checksum != hex.EncodeToString(sum[:]) || envelope.Report.SchemaVersion != 1 ||
		envelope.Report.Snapshot.TenantUUID != identity.TenantUUID ||
		envelope.Report.Snapshot.Env != identity.Env || envelope.Report.Snapshot.RunID != identity.RunID ||
		!sameReportIdentity(envelope.Report.Snapshot, identity) || !terminal(envelope.Report.Snapshot.Status) {
		return RunReport{}, ErrInvalid
	}
	return envelope.Report, nil
}

func reportObjectKey(identity Snapshot) string {
	return fmt.Sprintf("agent-runs/%s/%s/%s/report-v1.json", identity.TenantUUID, identity.Env, identity.RunID)
}

func sameReportIdentity(a, b Snapshot) bool {
	return a.TenantUUID == b.TenantUUID && a.Env == b.Env && a.RunID == b.RunID &&
		a.SessionID == b.SessionID && a.MessageID == b.MessageID && a.TraceID == b.TraceID && a.DeadlineAt.Equal(b.DeadlineAt)
}
