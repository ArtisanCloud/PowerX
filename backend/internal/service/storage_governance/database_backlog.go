package storagegov

import (
	"context"
	"encoding/json"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"time"
)

// DatabaseBacklog 是有界存在性探测，不是全表计数，也不将 pending 业务事件当作过期日志。
type DatabaseBacklog struct {
	OwnerInstance     string     `json:"owner_instance"`
	State             string     `json:"state"`
	ObservedAt        *time.Time `json:"observed_at"`
	Since             *time.Time `json:"since"`
	EligiblePresent   bool       `json:"eligible_present"`
	QuarantinePresent bool       `json:"quarantine_present"`
	PolicyRevision    int64      `json:"policy_revision"`
}

func (s *Service) databaseBacklog(ctx context.Context) (DatabaseBacklog, error) {
	out := DatabaseBacklog{OwnerInstance: s.opts.Identity.InstanceID, State: "not_checked"}
	if s.opts.Budget == nil {
		return out, nil
	}
	raw, e := s.opts.Budget.Document(ctx, "database-backlog", nil)
	if e != nil {
		return out, e
	}
	if len(raw) > 0 && (json.Unmarshal(raw, &out) != nil || out.OwnerInstance != s.opts.Identity.InstanceID) {
		return out, ErrStateUnavailable
	}
	return out, nil
}

// 只有后台观察周期写探测状态，HTTP 读取不触发 SQL 扫描或状态写入。
func (s *Service) observeDatabaseBacklog(ctx context.Context) {
	if s.opts.Budget == nil || s.dbRetention == nil {
		return
	}
	monitor, _, e := s.monitoringControl(ctx)
	if e != nil || !monitor.Enabled {
		return
	}
	p, configuration, policyError := s.loadRetentionControl(ctx)
	now := s.opts.Now().UTC()
	out := DatabaseBacklog{OwnerInstance: s.opts.Identity.InstanceID, State: "disabled", ObservedAt: &now, PolicyRevision: p.Revision}
	if policyError != nil {
		out.State = "unavailable"
	} else if p.Enabled && configuration.ExecutionReady {
		run, cancel := context.WithTimeout(ctx, 2*time.Second)
		e = s.dbRetention.Ready(run)
		var counts caprepo.RetentionBatch
		if e == nil {
			counts, e = s.dbRetention.Batch(run, caprepo.RetentionCutoffs{Success: now.AddDate(0, 0, -p.SuccessDays), Failure: now.AddDate(0, 0, -p.FailureDays), Pending: now.Add(-time.Duration(p.PendingHours) * time.Hour), Quarantine: now.AddDate(0, 0, -p.QuarantineDays), Limit: 1, AdoptLegacy: p.AdoptLegacy}, false)
		}
		cancel()
		out.State = "observed"
		out.EligiblePresent = counts.Traces+counts.Events > 0
		out.QuarantinePresent = counts.Quarantined > 0
		if e != nil {
			out.State = "unavailable"
			out.EligiblePresent = false
			out.QuarantinePresent = false
		}
	}
	_, _ = s.opts.Budget.Document(ctx, "database-backlog", func(raw []byte) ([]byte, error) {
		var old DatabaseBacklog
		if len(raw) > 0 && (json.Unmarshal(raw, &old) != nil || old.OwnerInstance != out.OwnerInstance) {
			return nil, ErrStateUnavailable
		}
		if out.EligiblePresent {
			out.Since = &now
			if old.State == "observed" && old.EligiblePresent && old.Since != nil && old.PolicyRevision == out.PolicyRevision && old.ObservedAt != nil && now.Sub(*old.ObservedAt) <= 2*SampleInterval {
				out.Since = old.Since
			}
		}
		return json.Marshal(out)
	})
}
func evaluateDatabaseBacklog(now time.Time, p TableMonitoringPolicy, b DatabaseBacklog) []CapacityAlert {
	out := []CapacityAlert{}
	if b.State == "disabled" {
		return out
	}
	if b.State == "unavailable" {
		return append(out, CapacityAlert{Code: "database_backlog_unavailable", Level: "unknown"})
	}
	if b.ObservedAt == nil || now.Sub(*b.ObservedAt) > time.Duration(p.StaleAfterMinutes)*time.Minute {
		return append(out, CapacityAlert{Code: "database_backlog_stale", Level: "unknown"})
	}
	if b.EligiblePresent && b.Since != nil && now.Sub(*b.Since) >= time.Duration(p.StaleAfterMinutes)*time.Minute {
		out = append(out, CapacityAlert{Code: "database_cleanup_backlog", Level: "warning"})
	}
	return out
}
