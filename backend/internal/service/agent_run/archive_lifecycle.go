package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrArchiveBackpressure = errors.New("archive.backpressure")
	ErrArchiveHealthStale  = errors.New("archive.health_stale")
)

// ArchivePolicy 控制已校验归档的热数据保留与新运行受理阈值。
type ArchivePolicy struct {
	HotRetention time.Duration
	MaxPending   int64
	MaxAge       time.Duration
	HealthTTL    time.Duration
}

func (p ArchivePolicy) Validate() error {
	if p.HotRetention < time.Hour || p.HotRetention > 90*24*time.Hour || p.MaxPending < 1 || p.MaxAge < time.Minute || p.MaxAge > 7*24*time.Hour || p.HealthTTL < 3*time.Second || p.HealthTTL > 5*time.Minute {
		return ErrInvalid
	}
	return nil
}

type ArchiveHealth struct {
	Policy     string     `json:"policy"`
	Pending    int64      `json:"pending"`
	OldestAt   *time.Time `json:"oldest_at,omitempty"`
	ObservedAt time.Time  `json:"observed_at"`
	Blocked    bool       `json:"blocked"`
	Reason     string     `json:"reason,omitempty"`
}

func archiveHealthKey(env string) string { return "agent:archive:{" + env + "}:health" }
func validArchiveEnv(env string) bool {
	return strings.TrimSpace(env) == env && len(env) > 0 && len(env) <= 32 && !strings.ContainsAny(env, "{}:")
}

func (s *RedisStore) PublishArchiveHealth(ctx context.Context, env string, health ArchiveHealth, policy ArchivePolicy) error {
	if s == nil || s.client == nil || !validArchiveEnv(env) || policy.Validate() != nil || health.Pending < 0 {
		return ErrInvalid
	}
	health.Policy = archivePolicyID(policy)
	health.ObservedAt = s.clock().UTC()
	health.Blocked = health.Pending >= policy.MaxPending || (health.OldestAt != nil && !health.ObservedAt.Before(health.OldestAt.Add(policy.MaxAge)))
	if health.Blocked {
		health.Reason = ErrArchiveBackpressure.Error()
	} else {
		health.Reason = ""
	}
	raw, err := json.Marshal(health)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, archiveHealthKey(env), raw, policy.HealthTTL).Err()
}

func (s *RedisStore) CheckArchiveAdmission(ctx context.Context, env string, policy ArchivePolicy) error {
	if s == nil || s.client == nil || !validArchiveEnv(env) || policy.Validate() != nil {
		return ErrInvalid
	}
	raw, err := s.client.Get(ctx, archiveHealthKey(env)).Bytes()
	if err != nil {
		return fmt.Errorf("%w: health unavailable", ErrArchiveHealthStale)
	}
	var health ArchiveHealth
	if json.Unmarshal(raw, &health) != nil || health.Policy != archivePolicyID(policy) || health.ObservedAt.IsZero() || s.clock().Before(health.ObservedAt) || !s.clock().Before(health.ObservedAt.Add(policy.HealthTTL)) {
		return ErrArchiveHealthStale
	}
	if health.Blocked {
		return ErrArchiveBackpressure
	}
	return nil
}

// ExpireArchivedRun 只为已归档终态设置同一绝对到期时间；调用方先持久化 SQL locator。
// 回执位于 tasks hash，一并归档后才可到期；不对活跃任务或共享队列设置 TTL。
func (s *RedisStore) ExpireArchivedRun(ctx context.Context, identity Snapshot, objects ReportObjectStore, at time.Time) error {
	if s == nil || s.client == nil || !validScope(identity) || at.IsZero() {
		return ErrInvalid
	}
	report, err := ReadRunReport(ctx, identity, objects)
	if err != nil {
		return err
	}
	state, events := runKeys(identity)
	plans, tasks, outbox := schedulingKeys(identity)
	for attempt := 0; attempt < 5; attempt++ {
		err = s.client.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.HGetAll(ctx, state).Result()
			if err != nil {
				return err
			}
			if len(values) == 0 {
				if !s.clock().Before(at) {
					return nil
				}
				return ErrNotFound
			}
			current, err := decodeSnapshot(values)
			if err != nil {
				return err
			}
			if !terminal(current.Status) || !sameReportIdentity(current, identity) || current.ArchiveKey != reportObjectKey(identity) || current.ArchivedAt.IsZero() || current.Version != report.Snapshot.Version || current.EventSeq != report.Snapshot.EventSeq || !at.After(current.ArchivedAt) {
				return ErrConflict
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				for _, key := range []string{state, events, plans, tasks, outbox, strings.TrimSuffix(outbox, ":outbox") + ":dispatch_cursor"} {
					pipe.ExpireAt(ctx, key, at)
				}
				return nil
			})
			return err
		}, state)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return err
	}
	return ErrConflict
}

func archivePolicyID(p ArchivePolicy) string {
	return fmt.Sprintf("%d:%d:%d:%d", p.HotRetention, p.MaxPending, p.MaxAge, p.HealthTTL)
}
