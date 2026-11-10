package storagegov

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"gorm.io/datatypes"
)

const SampleMetricVersion = "allocated-occupancy/v1"
const SampleInterval = 5 * time.Minute

// SamplingStatus 说明配置和 worker 准入，不把它们伪装成“正在运行”或已有完整历史。
type SamplingStatus struct {
	Configured            bool `json:"configured"`
	WorkerEnabled         bool `json:"worker_enabled"`
	IntervalSeconds       int  `json:"interval_seconds"`
	MetadataRetentionDays int  `json:"metadata_retention_days"`
}

func (s *Service) samplingStatus() SamplingStatus {
	return SamplingStatus{Configured: s.opts.SamplingConfigured, WorkerEnabled: s.opts.SamplingConfigured && s.opts.SamplingWorkerEnabled, IntervalSeconds: int(SampleInterval / time.Second), MetadataRetentionDays: 15}
}

type HistoryPoint struct {
	ObservedAt      time.Time        `json:"observed_at"`
	FilesystemID    string           `json:"filesystem_id,omitempty"`
	UsedBytes       *int64           `json:"used_bytes"`
	AvailableBytes  *int64           `json:"available_bytes"`
	DatabaseBytes   *int64           `json:"database_bytes"`
	Coverage        string           `json:"coverage"`
	DatabaseProblem string           `json:"database_problem,omitempty"`
	Resources       []ResourceSample `json:"resources"`
}

// ResourceSample 只记录有界摘要；NetChangeBytes 是相邻完整观测的净变化，不冒充写入量。
type ResourceSample struct {
	ID             string `json:"id"`
	Fingerprint    string `json:"fingerprint"`
	State          string `json:"state"`
	Bytes          *int64 `json:"bytes"`
	Files          int64  `json:"files"`
	Problem        string `json:"problem,omitempty"`
	NetChangeBytes *int64 `json:"net_change_bytes"`
}

func sourceFingerprint(r Resource, fs string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(r.ID+"\x00"+r.Kind+"\x00"+r.Scope+"\x00"+r.InstanceID+"\x00"+r.Path+"\x00"+fs+fmt.Sprint(r.LogActiveFiles))))
}

// 仅持久化页面实际需要的摘要，避免每槽复制目录清单导致观测自身膨胀。
type sampleDocument struct {
	Identity   Identity         `json:"identity"`
	ObservedAt time.Time        `json:"observed_at"`
	Filesystem Filesystem       `json:"filesystem"`
	Database   *sampleDatabase  `json:"database,omitempty"`
	Coverage   string           `json:"coverage"`
	Resources  []ResourceSample `json:"resources,omitempty"`
}
type sampleDatabase struct {
	DatabaseBytes    *int64                `json:"database_bytes"`
	Problem          string                `json:"problem,omitempty"`
	TableFingerprint string                `json:"table_fingerprint,omitempty"`
	Tables           []DatabaseTableSample `json:"tables,omitempty"`
}

// ObservationHistory 不把占用差值解释为清理前新增量，也不冒充文件／远端真实历史覆盖。
type ObservationHistory struct {
	MetricVersion       string         `json:"metric_version"`
	WindowDays          int            `json:"window_days"`
	SampleCount         int            `json:"sample_count"`
	ExpectedSamples     int            `json:"expected_samples"`
	MissingSamples      int            `json:"missing_samples"`
	MetricChanges       int            `json:"metric_changes"`
	UnavailableSamples  int            `json:"unavailable_samples"`
	State               string         `json:"state"`
	RateBasis           string         `json:"rate_basis"`
	DailyIncrementBytes *int64         `json:"daily_increment_bytes"`
	Points              []HistoryPoint `json:"points"`
	Sampling            SamplingStatus `json:"sampling"`
	Sources             []ResourceRef  `json:"sources"`
}

// CaptureSample 只写本模块有限的观测元数据，无文件删除／压缩／业务表写入。
func (s *Service) CaptureSample(ctx context.Context) error {
	if !s.stateReady() {
		return ErrStateUnavailable
	}
	// 页面缓存不能让新时间槽复用上一槽的旧观测，否则看似五分钟采样会产生隐性缺段。
	s.mu.Lock()
	if s.cached != nil && s.cached.ObservedAt.Truncate(SampleInterval) != s.opts.Now().UTC().Truncate(SampleInterval) {
		s.cached = nil
	}
	s.mu.Unlock()
	out, err := s.Overview(ctx)
	if err != nil {
		return err
	}
	record := sampleDocument{Identity: out.Identity, ObservedAt: out.ObservedAt, Filesystem: out.Filesystem, Coverage: out.Coverage}
	if out.Database != nil {
		record.Database = &sampleDatabase{DatabaseBytes: out.Database.DatabaseBytes, Problem: out.Database.Problem}
		if out.Database.DatabaseBytes == nil && record.Database.Problem == "" {
			record.Database.Problem = "database_unavailable"
		}
		if collector, ok := s.opts.DatabaseCollector.(SystemTableMetricsCollector); ok {
			items, e := collector.CollectSystemTableMetrics(ctx)
			if e != nil {
				return ErrStateUnavailable
			}
			if len(items) > 96 {
				return ErrStateUnavailable
			}
			seen := map[string]bool{}
			schema, databaseIdentity := "", ""
			relations := []string{}
			for _, item := range items {
				if !registeredSystemTable(item.Name) || seen[item.Name] || item.State != "observed" && item.State != "not_present" && item.State != "unavailable" || item.State == "observed" && item.TotalBytes == nil || item.State != "observed" && item.TotalBytes != nil || item.TotalBytes != nil && (*item.TotalBytes < 0 || *item.TotalBytes > maxSafeBytes) {
					return ErrStateUnavailable
				}
				if len(seen) > 0 && (schema != item.Schema || databaseIdentity != item.DatabaseIdentity) {
					return ErrStateUnavailable
				}
				seen[item.Name] = true
				schema = item.Schema
				databaseIdentity = item.DatabaseIdentity
				relations = append(relations, fmt.Sprintf("%s/%d", item.Name, item.RelationID))
				record.Database.Tables = append(record.Database.Tables, DatabaseTableSample{ID: item.Name, State: item.State, Bytes: item.TotalBytes})
			}
			sort.Strings(relations)
			record.Database.TableFingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(out.Database.ClusterID+"\x00"+databaseIdentity+"\x00"+schema+"\x00"+strings.Join(relations, "\x00")+"\x00allocated-system-tables/v1")))
		}
	}
	var basis string
	for _, r := range out.Resources {
		basis += sourceFingerprint(r.Resource, r.FilesystemID) + r.State
	}
	for _, r := range out.Resources {
		record.Resources = append(record.Resources, ResourceSample{ID: r.ID, Fingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte(sourceFingerprint(r.Resource, r.FilesystemID)+basis))), State: r.State, Bytes: r.AttributedBytes, Files: r.Files, Problem: r.Problem})
	}
	raw, err := json.Marshal(record)
	if err != nil || len(raw) > 65536 {
		return ErrStateUnavailable
	}
	row := &m.Sample{TenantID: 0, InstanceID: s.opts.Identity.InstanceID, Slot: out.ObservedAt.Truncate(SampleInterval), MetricVersion: SampleMetricVersion, Document: datatypes.JSON(raw)}
	if s.store.SaveSample(ctx, row) != nil {
		return ErrStateUnavailable
	}
	if record.Database != nil && len(record.Database.Tables) > 0 {
		s.PublishChange("database_tables_sampled")
	}
	return nil
}

func (s *Service) History(ctx context.Context) (ObservationHistory, error) {
	out := ObservationHistory{MetricVersion: SampleMetricVersion, WindowDays: 14, ExpectedSamples: 14 * 24 * 12, State: "sampling_required", RateBasis: "ingress_not_instrumented", Points: []HistoryPoint{}, Sources: []ResourceRef{}, Sampling: s.samplingStatus()}
	for _, r := range s.opts.Resources {
		out.Sources = append(out.Sources, refFor(r))
	}
	if !s.stateReady() {
		return out, ErrStateUnavailable
	}
	now := s.opts.Now().UTC()
	start := now.Truncate(SampleInterval).Add(-14 * 24 * time.Hour)
	rows, err := s.store.Samples(ctx, s.opts.Identity.InstanceID, start)
	if err != nil {
		return out, ErrStateUnavailable
	}
	previousFilesystem := ""
	previousResources := map[string]ResourceSample{}
	var previousTime time.Time
	for _, row := range rows {
		// 当前未完成五分钟槽不参与预期完整样本，时间范围始终为 [start, end)。
		if !row.Slot.Before(now.Truncate(SampleInterval)) {
			continue
		}
		if row.MetricVersion != SampleMetricVersion {
			out.MetricChanges++
			continue
		}
		var snap sampleDocument
		if json.Unmarshal(row.Document, &snap) != nil || snap.Identity != s.opts.Identity {
			out.MetricChanges++
			continue
		}
		if snap.Filesystem.ID != "" {
			if previousFilesystem != "" && snap.Filesystem.ID != previousFilesystem {
				out.MetricChanges++
			}
			previousFilesystem = snap.Filesystem.ID
		}
		point := HistoryPoint{ObservedAt: snap.ObservedAt, FilesystemID: snap.Filesystem.ID, UsedBytes: snap.Filesystem.UsedBytes, AvailableBytes: snap.Filesystem.AvailableBytes, Coverage: snap.Coverage}
		if snap.Database != nil {
			point.DatabaseBytes = snap.Database.DatabaseBytes
			point.DatabaseProblem = snap.Database.Problem
		}
		if point.DatabaseBytes == nil && point.DatabaseProblem == "" {
			point.DatabaseProblem = "not_recorded"
		}
		point.Resources = append([]ResourceSample{}, snap.Resources...)
		for i := range point.Resources {
			r := &point.Resources[i]
			r.NetChangeBytes = nil
			p, exists := previousResources[r.ID]
			if exists && r.Fingerprint == p.Fingerprint && r.State == "observed" && p.State == "observed" && r.Bytes != nil && p.Bytes != nil && !previousTime.IsZero() && snap.ObservedAt.Sub(previousTime) > 0 && snap.ObservedAt.Sub(previousTime) <= 2*SampleInterval {
				delta := *r.Bytes - *p.Bytes
				r.NetChangeBytes = &delta
			}
		}
		previousResources = map[string]ResourceSample{}
		for _, r := range point.Resources {
			previousResources[r.ID] = r
		}
		previousTime = snap.ObservedAt
		if snap.Filesystem.State != "observed" {
			out.UnavailableSamples++
		}
		out.Points = append(out.Points, point)
	}
	out.SampleCount = len(out.Points)
	out.MissingSamples = max(0, out.ExpectedSamples-out.SampleCount)
	if out.SampleCount > 0 {
		out.State = "occupancy_only"
	}
	return out, nil
}

// StartSampling 是后台观测定时器，不消费私有任务队列；只在部署显式开启后启动。
// onError 使用固定诊断，调用方应避免输出数据库连接信息。
func (s *Service) StartSampling(ctx context.Context, onError func(error)) {
	go func() {
		capture := func() {
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := s.CaptureSample(run); err != nil && ctx.Err() == nil && onError != nil {
				onError(err)
			}
		}
		capture()
		tick := time.NewTicker(SampleInterval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				capture()
			}
		}
	}()
}
