package storagegov

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
)

// SystemTableMetricsCollector 只采后台容量元数据，不读取记录载荷或时间范围。
type SystemTableMetricsCollector interface {
	CollectSystemTableMetrics(context.Context) ([]repo.SystemTableObservation, error)
}

// DatabaseTableSample 是每个采样槽的单表有界摘要，未知空间使用 nil。
type DatabaseTableSample struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Bytes *int64 `json:"bytes"`
}

// TableMonitoringPolicy 是 Root 明确保存的实例级告警阈值，默认停用。
type TableMonitoringPolicy struct {
	Revision            int64     `json:"revision"`
	OwnerInstance       string    `json:"owner_instance"`
	Enabled             bool      `json:"enabled"`
	WarningBytes        int64     `json:"warning_bytes"`
	GrowthBytes         int64     `json:"growth_bytes"`
	GrowthWindowMinutes int       `json:"growth_window_minutes"`
	StaleAfterMinutes   int       `json:"stale_after_minutes"`
	FailureLimit        int       `json:"failure_limit"`
	UpdatedAt           time.Time `json:"updated_at"`
	Actor               uint64    `json:"actor"`
}

// TableHealth 表达当前样本、实际基线及净占用变化。
type TableHealth struct {
	DatabaseTableSample
	ObservedAt     time.Time  `json:"observed_at"`
	BaselineAt     *time.Time `json:"baseline_at"`
	NetChangeBytes *int64     `json:"net_change_bytes"`
	Level          string     `json:"level"`
	Problem        string     `json:"problem,omitempty"`
}

// TableMonitoringStatus 区分配置、样本和清理执行健康。
type TableMonitoringStatus struct {
	State              string                        `json:"state"`
	StateConfigured    bool                          `json:"state_configured"`
	SamplingConfigured bool                          `json:"sampling_configured"`
	WorkerEnabled      bool                          `json:"worker_enabled"`
	Policy             TableMonitoringPolicy         `json:"policy"`
	SampledAt          *time.Time                    `json:"sampled_at"`
	Tables             []TableHealth                 `json:"tables"`
	Alerts             []CapacityAlert               `json:"alerts"`
	Cleanup            *CleanupHealth                `json:"cleanup"`
	Backlog            *DatabaseBacklog              `json:"backlog"`
	Problem            string                        `json:"problem,omitempty"`
	Configuration      *TableMonitoringConfiguration `json:"configuration"`
}

// TableHistoryPoint 是固定目录中一张表的占用时间点。
type TableHistoryPoint struct {
	ObservedAt time.Time `json:"observed_at"`
	State      string    `json:"state"`
	Bytes      *int64    `json:"bytes"`
}

// TableHistory 限定最近二十四小时，不将净变化冒充写入流量。
type TableHistory struct {
	ID          string              `json:"id"`
	WindowHours int                 `json:"window_hours"`
	Metric      string              `json:"metric"`
	Points      []TableHistoryPoint `json:"points"`
}

func registeredSystemTable(id string) bool {
	for _, d := range m.SystemTableCatalog() {
		if d.Name == id {
			return true
		}
	}
	return false
}
func (s *Service) tableMonitoringPolicy(raw []byte) (TableMonitoringPolicy, error) {
	p := TableMonitoringPolicy{OwnerInstance: s.opts.Identity.InstanceID, WarningBytes: 4 << 30, GrowthBytes: 256 << 20, GrowthWindowMinutes: 15, StaleAfterMinutes: 45, FailureLimit: 3}
	if len(raw) > 0 && (json.Unmarshal(raw, &p) != nil || p.OwnerInstance != s.opts.Identity.InstanceID || !validTableMonitoringPolicy(p)) {
		return p, ErrStateUnavailable
	}
	return p, nil
}
func validTableMonitoringPolicy(p TableMonitoringPolicy) bool {
	return p.WarningBytes >= 64<<20 && p.WarningBytes <= 1<<50 && p.GrowthBytes >= 1<<20 && p.GrowthBytes <= 1<<50 && p.GrowthWindowMinutes >= 5 && p.GrowthWindowMinutes <= 1440 && p.StaleAfterMinutes >= 15 && p.StaleAfterMinutes <= 1440 && p.FailureLimit >= 1 && p.FailureLimit <= 20
}

// SaveTableMonitoring 保留旧调用的明确升级错误，禁止继续写旧 JSON。
func (s *Service) SaveTableMonitoring(ctx context.Context, revision int64, p TableMonitoringPolicy) (TableMonitoringPolicy, error) {
	if rootActor(ctx) == 0 {
		return p, ErrRootRequired
	}
	return p, controlError(428, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
}

func (s *Service) tableSnapshots(ctx context.Context) ([]sampleDocument, error) {
	if !s.stateReady() {
		return nil, ErrStateUnavailable
	}
	now := s.opts.Now().UTC()
	rows, err := s.store.Samples(ctx, s.opts.Identity.InstanceID, now.Add(-24*time.Hour))
	if err != nil {
		return nil, ErrStateUnavailable
	}
	out := []sampleDocument{}
	for _, row := range rows {
		if row.MetricVersion != SampleMetricVersion || len(row.Document) > 65536 {
			continue
		}
		var d sampleDocument
		if json.Unmarshal(row.Document, &d) != nil || d.Identity != s.opts.Identity || d.Database == nil || d.Database.TableFingerprint == "" || len(d.Database.Tables) > 96 || d.ObservedAt.After(now) || d.ObservedAt.Before(now.Add(-24*time.Hour)) {
			continue
		}
		valid := d.ObservedAt.Truncate(SampleInterval).Equal(row.Slot)
		seen := map[string]bool{}
		for _, v := range d.Database.Tables {
			if !registeredSystemTable(v.ID) || seen[v.ID] || v.State != "observed" && v.State != "not_present" && v.State != "unavailable" || v.State == "observed" && v.Bytes == nil || v.State != "observed" && v.Bytes != nil || v.Bytes != nil && (*v.Bytes < 0 || *v.Bytes > maxSafeBytes) {
				valid = false
				break
			}
			seen[v.ID] = true
		}
		if !valid {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ObservedAt.Before(out[j].ObservedAt) })
	if len(out) > 289 {
		out = out[len(out)-289:]
	}
	return out, nil
}

// EvaluateTableSnapshots 只比较同一计量口径、连续采样中的净占用变化，不冒充真实写入流量。
func EvaluateTableSnapshots(now time.Time, p TableMonitoringPolicy, samples []sampleDocument) ([]TableHealth, []CapacityAlert, *time.Time) {
	rows := []TableHealth{}
	alerts := []CapacityAlert{}
	if len(samples) == 0 {
		return rows, alerts, nil
	}
	latest := samples[len(samples)-1]
	if latest.Database == nil {
		return rows, alerts, nil
	}
	at := latest.ObservedAt
	var baseline *sampleDocument
	target := at.Add(-time.Duration(p.GrowthWindowMinutes) * time.Minute)
	for i := len(samples) - 2; i >= 0; i-- {
		candidate := &samples[i]
		if candidate.ObservedAt.After(target) {
			continue
		}
		if target.Sub(candidate.ObservedAt) <= SampleInterval && candidate.Database != nil && candidate.Database.TableFingerprint == latest.Database.TableFingerprint {
			baseline = candidate
		}
		break
	}
	for _, item := range latest.Database.Tables {
		row := TableHealth{DatabaseTableSample: item, ObservedAt: at, Level: "normal"}
		if item.State == "observed" && item.Bytes != nil && baseline != nil {
			continuous := true
			var previous time.Time
			for _, snapshot := range samples {
				if snapshot.ObservedAt.Before(baseline.ObservedAt) {
					continue
				}
				if snapshot.Database == nil || snapshot.Database.TableFingerprint != latest.Database.TableFingerprint || !previous.IsZero() && snapshot.ObservedAt.Sub(previous) > 2*SampleInterval {
					continuous = false
					break
				}
				found := false
				for _, entry := range snapshot.Database.Tables {
					if entry.ID == item.ID && entry.State == "observed" && entry.Bytes != nil {
						found = true
						break
					}
				}
				if !found {
					continuous = false
					break
				}
				previous = snapshot.ObservedAt
			}
			if continuous {
				for _, base := range baseline.Database.Tables {
					if base.ID == item.ID && base.State == "observed" && base.Bytes != nil {
						delta := *item.Bytes - *base.Bytes
						row.NetChangeBytes = &delta
						t := baseline.ObservedAt
						row.BaselineAt = &t
						break
					}
				}
			}
		}
		code := ""
		switch {
		case item.State == "unavailable" || item.State == "observed" && item.Bytes == nil:
			row.Level = "unknown"
			row.Problem = "table_metrics_unavailable"
			consecutive := 0
			previous := at
			for i := len(samples) - 1; i >= 0; i-- {
				if samples[i].Database == nil || samples[i].Database.TableFingerprint != latest.Database.TableFingerprint || previous.Sub(samples[i].ObservedAt) > 2*SampleInterval {
					break
				}
				bad := false
				for _, entry := range samples[i].Database.Tables {
					if entry.ID == item.ID && entry.State == "unavailable" {
						bad = true
					}
				}
				if !bad {
					break
				}
				consecutive++
				previous = samples[i].ObservedAt
			}
			if consecutive >= p.FailureLimit {
				code = "table_collection_failed"
			}
		case item.State == "not_present":
			row.Level = "unknown"
			row.Problem = "table_not_present"
		case row.NetChangeBytes != nil && *row.NetChangeBytes >= p.GrowthBytes:
			row.Level = "warning"
			code = "table_growth_abnormal"
		case item.Bytes != nil && *item.Bytes >= p.WarningBytes:
			row.Level = "warning"
			code = "table_capacity_warning"
		}
		if p.Enabled && code != "" {
			row.Problem = code
			alerts = append(alerts, CapacityAlert{Code: code, Level: "warning", ResourceID: item.ID})
		}
		if !p.Enabled && row.Level == "warning" {
			row.Level = "normal"
		}
		rows = append(rows, row)
	}
	return rows, alerts, &at
}

func (s *Service) tableMonitoring(ctx context.Context) TableMonitoringStatus {
	p, _ := s.tableMonitoringPolicy(nil)
	out := TableMonitoringStatus{State: "disabled", StateConfigured: s.opts.Budget != nil, SamplingConfigured: s.opts.SamplingConfigured, WorkerEnabled: s.opts.SamplingConfigured && s.opts.SamplingWorkerEnabled, Policy: p, Tables: []TableHealth{}, Alerts: []CapacityAlert{}}
	if s.opts.Control != nil {
		var configuration TableMonitoringConfiguration
		var err error
		p, configuration, err = s.monitoringControl(ctx)
		out.Configuration = &configuration
		out.Policy = p
		if err != nil {
			out.State = "unavailable"
			out.Problem = configuration.Problem
			out.Alerts = append(out.Alerts, CapacityAlert{Code: out.Problem, Level: "unknown"})
			return out
		}
	} else {
		if s.opts.Budget != nil {
			raw, err := s.opts.Budget.Document(ctx, "table-monitoring", nil)
			if err != nil {
				out.State = "unavailable"
				out.Problem = "table_monitoring_state_unavailable"
				out.Alerts = append(out.Alerts, CapacityAlert{Code: out.Problem, Level: "unknown"})
				return out
			}
			p, err = s.tableMonitoringPolicy(raw)
			if err != nil {
				out.State = "unavailable"
				out.Problem = "table_monitoring_state_unavailable"
				out.Alerts = append(out.Alerts, CapacityAlert{Code: out.Problem, Level: "unknown"})
				return out
			}
			out.Policy = p
		}
		configuration := TableMonitoringConfiguration{Authority: "legacy", State: "legacy", LegacyRevision: p.Revision, LegacyEnabled: p.Enabled, Problem: "STORAGE_CONTROL_SCHEMA_NOT_READY"}
		out.Configuration = &configuration
	}
	if !s.opts.SamplingConfigured {
		if p.Enabled {
			out.State = "unavailable"
			out.Alerts = append(out.Alerts, CapacityAlert{Code: "table_sampling_worker_disabled", Level: "warning"})
		}
		out.Problem = "table_sampling_not_configured"
		return out
	}
	samples, err := s.tableSnapshots(ctx)
	if err != nil {
		out.Problem = "table_samples_unavailable"
	} else {
		out.Tables, out.Alerts, out.SampledAt = EvaluateTableSnapshots(s.opts.Now().UTC(), p, samples)
	}
	if !p.Enabled {
		return out
	}
	if err != nil {
		out.Alerts = append(out.Alerts, CapacityAlert{Code: "table_samples_unavailable", Level: "unknown"})
	}
	now := s.opts.Now().UTC()
	out.State = "observed"
	switch {
	case !out.WorkerEnabled:
		out.State = "unavailable"
		out.Alerts = append(out.Alerts, CapacityAlert{Code: "table_sampling_worker_disabled", Level: "warning"})
	case out.SampledAt == nil:
		out.State = "sampling_required"
		if !p.UpdatedAt.IsZero() && now.Sub(p.UpdatedAt) > time.Duration(p.StaleAfterMinutes)*time.Minute {
			out.State = "stale"
			out.Alerts = append(out.Alerts, CapacityAlert{Code: "table_sampling_stalled", Level: "warning"})
		}
	case now.Sub(*out.SampledAt) > time.Duration(p.StaleAfterMinutes)*time.Minute:
		out.State = "stale"
		out.Alerts = append(out.Alerts, CapacityAlert{Code: "table_sampling_stalled", Level: "warning"})
	}
	if out.State == "observed" {
		count := 0
		for _, row := range out.Tables {
			if row.State == "observed" && row.Bytes != nil {
				count++
			}
		}
		if count == 0 {
			out.State = "unavailable"
			out.Problem = "table_metrics_unavailable"
			out.Alerts = append(out.Alerts, CapacityAlert{Code: out.Problem, Level: "unknown"})
		}
	}
	health, healthErr := s.cleanupHealth(ctx)
	if healthErr != nil {
		out.Alerts = append(out.Alerts, CapacityAlert{Code: "cleanup_health_unavailable", Level: "unknown"})
	} else {
		out.Cleanup = &health
		out.Alerts = append(out.Alerts, evaluateCleanupHealth(now, p, health, s.opts.LifecycleWorkerEnabled)...)
	}
	if s.opts.Budget == nil {
		return out
	}
	if s.opts.Control != nil {
		policy, configuration, e := s.loadRetentionControl(ctx)
		if e != nil || policy.Enabled && !configuration.ExecutionReady {
			out.Alerts = append(out.Alerts, CapacityAlert{Code: "database_backlog_unavailable", Level: "unknown"})
		} else if policy.Enabled && (!s.opts.DatabaseRetentionAllowed || !s.opts.LifecycleWorkerEnabled) {
			out.Alerts = append(out.Alerts, CapacityAlert{Code: "database_cleanup_worker_disabled", Level: "warning"})
		}
	}
	if legacy, _, e := s.legacyRetention(ctx); e == nil {
		for _, r := range legacy.Runs {
			if r.State == "unconfirmed" {
				out.Alerts = append(out.Alerts, CapacityAlert{Code: "database_cleanup_unconfirmed", Level: "warning"})
				break
			}
		}
	}
	if backlog, e := s.databaseBacklog(ctx); e == nil {
		out.Backlog = &backlog
		out.Alerts = append(out.Alerts, evaluateDatabaseBacklog(now, p, backlog)...)
	} else {
		out.Alerts = append(out.Alerts, CapacityAlert{Code: "database_backlog_unavailable", Level: "unknown"})
	}
	for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
		if s.opts.Control != nil {
			policy, configuration, e := s.loadSystemRetentionControl(ctx, id)
			if e != nil || policy.Enabled && !configuration.ExecutionReady {
				out.Alerts = append(out.Alerts, CapacityAlert{Code: "cleanup_health_unavailable", Level: "unknown", ResourceID: id})
			} else if policy.Enabled && (!s.opts.DatabaseRetentionAllowed || !s.opts.LifecycleWorkerEnabled) {
				out.Alerts = append(out.Alerts, CapacityAlert{Code: "database_cleanup_worker_disabled", Level: "warning", ResourceID: id})
			}
		}
		if legacy, _, e := s.legacySystemRetention(ctx, id); e == nil {
			for _, r := range legacy.Runs {
				if r.TableID == id && r.State == "unconfirmed" {
					out.Alerts = append(out.Alerts, CapacityAlert{Code: "database_cleanup_unconfirmed", Level: "warning", ResourceID: id})
					break
				}
			}
		}
	}
	return out
}

// DatabaseTableMonitoring 返回 Root 专用的当前监控状态。
func (s *Service) DatabaseTableMonitoring(ctx context.Context) (TableMonitoringStatus, error) {
	if rootActor(ctx) == 0 {
		return TableMonitoringStatus{}, ErrRootRequired
	}
	return s.tableMonitoring(ctx), nil
}

// DatabaseTableHistory 读取固定表目录的有界历史，不接受任意表。
func (s *Service) DatabaseTableHistory(ctx context.Context, id string) (TableHistory, error) {
	out := TableHistory{ID: id, WindowHours: 24, Metric: "allocated_bytes_net_change_not_ingress", Points: []TableHistoryPoint{}}
	if rootActor(ctx) == 0 {
		return out, ErrRootRequired
	}
	if !registeredSystemTable(id) {
		return out, ErrSystemTableNotFound
	}
	docs, err := s.tableSnapshots(ctx)
	if err != nil {
		return out, err
	}
	for _, d := range docs {
		for _, item := range d.Database.Tables {
			if item.ID == id {
				out.Points = append(out.Points, TableHistoryPoint{ObservedAt: d.ObservedAt, State: item.State, Bytes: item.Bytes})
				break
			}
		}
	}
	return out, nil
}
