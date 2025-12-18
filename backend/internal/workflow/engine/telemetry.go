package engine

import (
	"context"
	"sync/atomic"
	"time"

	capmetrics "github.com/ArtisanCloud/PowerX/internal/observability/metrics"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// WorkflowTelemetry 负责记录 Workflow Catalog 与执行链路的采纳度指标。
type WorkflowTelemetry struct {
	metrics    *capmetrics.CapabilityRegistryMetrics
	logger     *pxlog.Logger
	execTotal  atomic.Int64
	execFailed atomic.Int64
	catalog    atomic.Value // catalogSummary
}

// WorkflowTelemetrySnapshot 汇总最近一次的 Catalog 与执行指标。
type WorkflowTelemetrySnapshot struct {
	CatalogVersion   string
	TotalTemplates   int
	NeedsUpgrade     int
	LastGeneratedAt  time.Time
	ExecutionsTotal  int64
	ExecutionsFailed int64
}

type catalogSummary struct {
	Version     string
	Total       int
	Needs       int
	GeneratedAt time.Time
}

// WorkflowExecutionTelemetryInput 描述一次 Workflow 节点执行的指标输入。
type WorkflowExecutionTelemetryInput struct {
	CapabilityID string
	TemplateID   string
	PluginID     string
	TenantUUID   string
	Protocol     string
	Status       string
	NeedsUpgrade bool
	Err          error
}

// NewWorkflowTelemetry 构造 WorkflowTelemetry。
func NewWorkflowTelemetry(metrics *capmetrics.CapabilityRegistryMetrics) *WorkflowTelemetry {
	if metrics == nil {
		metrics = capmetrics.NewCapabilityRegistryMetrics(nil)
	}
	return &WorkflowTelemetry{
		metrics: metrics,
		logger:  pxlog.GetGlobalLogger(),
	}
}

// ObserveWorkflowCatalogSnapshot 实现 capability_registry.WorkflowCatalogTelemetry。
func (t *WorkflowTelemetry) ObserveWorkflowCatalogSnapshot(ctx context.Context, snapshot capservice.WorkflowCatalogSnapshot) {
	if t == nil {
		return
	}
	if len(snapshot.Templates) > 0 && t.metrics != nil {
		samples := make([]capmetrics.WorkflowCatalogSample, 0, len(snapshot.Templates))
		for _, tpl := range snapshot.Templates {
			samples = append(samples, capmetrics.WorkflowCatalogSample{
				TemplateID:   tpl.TemplateID,
				CapabilityID: tpl.CapabilityID,
				PluginID:     tpl.PluginID,
				NeedsUpgrade: tpl.RequiresManualUpgrade,
			})
		}
		t.metrics.ObserveWorkflowCatalog(ctx, samples)
	}

	needsUpgrade := 0
	for _, tpl := range snapshot.Templates {
		if tpl.RequiresManualUpgrade {
			needsUpgrade++
		}
	}
	t.catalog.Store(catalogSummary{
		Version:     snapshot.Version,
		Total:       len(snapshot.Templates),
		Needs:       needsUpgrade,
		GeneratedAt: snapshot.GeneratedAt,
	})

	if t.logger != nil {
		t.logger.InfoF(ctx, "[workflow.telemetry] catalog refreshed version=%s templates=%d needs_upgrade=%d", snapshot.Version, len(snapshot.Templates), needsUpgrade)
	}
}

// ObserveWorkflowExecution 记录 Workflow Engine 的执行采纳度。
func (t *WorkflowTelemetry) ObserveWorkflowExecution(ctx context.Context, input WorkflowExecutionTelemetryInput) {
	if t == nil {
		return
	}
	total := t.execTotal.Add(1)
	var failed int64
	if input.Err != nil {
		failed = t.execFailed.Add(1)
	} else {
		failed = t.execFailed.Load()
	}

	if t.metrics != nil {
		t.metrics.ObserveWorkflowExecution(ctx, capmetrics.WorkflowExecutionSample{
			TemplateID:   input.TemplateID,
			CapabilityID: input.CapabilityID,
			PluginID:     input.PluginID,
			TenantUUID:   input.TenantUUID,
			Protocol:     input.Protocol,
			Status:       input.Status,
			NeedsUpgrade: input.NeedsUpgrade,
			Err:          input.Err,
		})
	}
	if t.logger != nil {
		if input.Err != nil {
			t.logger.WarnF(ctx, "[workflow.telemetry] execution failed capability=%s template=%s tenant=%s err=%v (total=%d failed=%d)", input.CapabilityID, input.TemplateID, input.TenantUUID, input.Err, total, failed)
		} else {
			if input.NeedsUpgrade {
				t.logger.WarnF(ctx, "[workflow.telemetry] execution success but template requires manual upgrade capability=%s template=%s tenant=%s (total=%d failed=%d)", input.CapabilityID, input.TemplateID, input.TenantUUID, total, failed)
			} else {
				t.logger.InfoF(ctx, "[workflow.telemetry] execution success capability=%s template=%s tenant=%s (total=%d failed=%d)", input.CapabilityID, input.TemplateID, input.TenantUUID, total, failed)
			}
		}
	}
}

// Snapshot 返回最新的遥测摘要，便于测试与调试。
func (t *WorkflowTelemetry) Snapshot() WorkflowTelemetrySnapshot {
	if t == nil {
		return WorkflowTelemetrySnapshot{}
	}
	snapshot := WorkflowTelemetrySnapshot{
		ExecutionsTotal:  t.execTotal.Load(),
		ExecutionsFailed: t.execFailed.Load(),
	}
	if value := t.catalog.Load(); value != nil {
		if info, ok := value.(catalogSummary); ok {
			snapshot.CatalogVersion = info.Version
			snapshot.TotalTemplates = info.Total
			snapshot.NeedsUpgrade = info.Needs
			snapshot.LastGeneratedAt = info.GeneratedAt
		}
	}
	return snapshot
}
