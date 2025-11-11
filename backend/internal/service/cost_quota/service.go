package cost_quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const quotaCacheKey = "agent:modelhub:quota:%s:%s:%s"

// Service coordinates quota ledger persistence, usage ingest, and telemetry.
type Service struct {
	db    *gorm.DB
	cache cache.ICache
	audit audit.Service
	keys  *tenantkeys.TenantKeyService
	repo  *repo.CostQuotaLedgerRepository
	inst  *instrumentation.Instrumentation
	clock func() time.Time
}

type Options struct {
	shared.Options
	Ledgers *repo.CostQuotaLedgerRepository
}

const maxEnforcementHistory = 20

func NewService(opts Options) *Service {
	opts.Options.Normalize()
	r := opts.Ledgers
	if r == nil && opts.DB != nil {
		r = repo.NewCostQuotaLedgerRepository(opts.DB)
	}
	return &Service{
		db:    opts.DB,
		cache: opts.Cache,
		audit: opts.AuditSvc,
		keys:  opts.TenantKeySvc,
		repo:  r,
		inst:  opts.Instrumentation,
		clock: opts.Clock,
	}
}

type LedgerInput struct {
	TenantID          string
	ProviderID        *uuid.UUID
	BudgetPeriod      string
	QuotaLimit        float64
	DashboardScope    string
	SensitiveMetadata map[string]string
}

// EnsureLedger upserts quota configuration per tenant/provider scope.
func (s *Service) EnsureLedger(ctx context.Context, env string, input LedgerInput) (*model.CostQuotaLedger, error) {
	if s.repo == nil {
		return nil, errors.New("quota repository is not configured")
	}
	if strings.TrimSpace(input.TenantID) == "" {
		return nil, errors.New("tenant_id required")
	}
	if strings.TrimSpace(input.BudgetPeriod) == "" {
		return nil, errors.New("budget_period required")
	}
	sealed, err := s.sealMetadata(ctx, env, input.SensitiveMetadata)
	if err != nil {
		return nil, err
	}
	ledger := &model.CostQuotaLedger{
		Env:               env,
		TenantID:          strings.TrimSpace(input.TenantID),
		BudgetPeriod:      strings.TrimSpace(input.BudgetPeriod),
		ProviderProfileID: input.ProviderID,
		QuotaLimit:        input.QuotaLimit,
		DashboardScope:    strings.TrimSpace(input.DashboardScope),
		SealedMetadata:    sealed,
	}
	result, err := s.repo.UpsertScope(ctx, ledger)
	if err != nil {
		return nil, err
	}
	s.cacheSnapshot(ctx, result)
	s.emitAudit(ctx, "cost_quota.ledger.upsert", result, nil)
	return result, nil
}

type UsageReport struct {
	LedgerID          uuid.UUID
	UsageActual       float64
	AnomalyState      datatypes.JSONMap
	EnforcementState  datatypes.JSONMap
	SensitiveMetadata map[string]string
}

type UsageIngestEvent struct {
	CostUSD float64
	Tokens  uint64
}

type UsageIngestInput struct {
	TenantID     string
	ProviderID   *uuid.UUID
	BudgetPeriod string
	Events       []UsageIngestEvent
}

type EnforcementInput struct {
	TenantID    string
	ProviderID  *uuid.UUID
	Action      string
	Reason      string
	TicketID    string
	RequestedBy string
}

// ReportUsage updates usage + anomaly metadata and seals optional sensitive notes.
func (s *Service) ReportUsage(ctx context.Context, env string, report UsageReport) error {
	if s.repo == nil {
		return errors.New("quota repository is not configured")
	}
	sealed, err := s.sealMetadata(ctx, env, report.SensitiveMetadata)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateUsage(ctx, report.LedgerID, report.UsageActual, report.AnomalyState, report.EnforcementState, timePtr(s.clock())); err != nil {
		return err
	}
	if sealed != nil {
		if err := s.repo.UpdateSealedMetadata(ctx, report.LedgerID, sealed); err != nil {
			return err
		}
	}
	ledger, err := s.repo.GetByUUID(ctx, report.LedgerID)
	if err == nil && ledger != nil {
		s.cacheSnapshot(ctx, ledger)
		s.inst.RecordMetric(ctx, "agent.cost.usage_actual", report.UsageActual, map[string]string{
			"tenant_id":     ledger.TenantID,
			"budget_period": ledger.BudgetPeriod,
		})
		s.emitAudit(ctx, "cost_quota.usage.reported", ledger, nil)
	}
	return nil
}

// ProcessUsage ingests raw events and rolls them into the ledger usage totals.
func (s *Service) ProcessUsage(ctx context.Context, env string, input UsageIngestInput) (*model.CostQuotaLedger, error) {
	if s.repo == nil {
		return nil, errors.New("quota repository is not configured")
	}
	tenant := strings.TrimSpace(input.TenantID)
	if tenant == "" {
		return nil, errors.New("tenant_id required")
	}
	period := strings.TrimSpace(input.BudgetPeriod)
	if period == "" {
		period = "monthly"
	}
	ledger, err := s.ensureLedgerExists(ctx, env, tenant, input.ProviderID, period)
	if err != nil {
		return nil, err
	}
	var delta float64
	for _, evt := range input.Events {
		delta += evt.CostUSD
	}
	if delta == 0 {
		return ledger, nil
	}
	newUsage := ledger.UsageActual + delta
	anomaly := cloneJSONMap(ledger.AnomalyState)
	var lastAnomaly *time.Time
	if ledger.QuotaLimit > 0 && newUsage > ledger.QuotaLimit {
		if anomaly == nil {
			anomaly = datatypes.JSONMap{}
		}
		anomaly["status"] = "breached"
		anomaly["breach_at"] = s.clock().UTC().Format(time.RFC3339)
		last := s.clock()
		lastAnomaly = &last
		s.inst.RecordMetric(ctx, "agent.provider.cost.anomaly", 1, map[string]string{
			"tenant_id":   tenant,
			"provider_id": providerKey(input.ProviderID),
		})
	}
	if err := s.repo.UpdateUsage(ctx, ledger.UUID, newUsage, anomaly, nil, lastAnomaly); err != nil {
		return nil, err
	}
	ledger, err = s.repo.GetByUUID(ctx, ledger.UUID)
	if err != nil {
		return nil, err
	}
	if ledger == nil {
		return nil, errors.New("ledger missing after usage update")
	}

	labels := map[string]string{
		"tenant_id":     tenant,
		"provider_id":   providerKey(input.ProviderID),
		"budget_period": period,
	}
	s.inst.RecordMetric(ctx, "agent.provider.cost_usage", newUsage, labels)
	s.cacheSnapshot(ctx, ledger)
	s.emitAudit(ctx, "cost_quota.usage.reported", ledger, map[string]any{
		"usage_delta": delta,
	})
	return ledger, nil
}

// ListLedgers returns quota ledgers for a tenant.
func (s *Service) ListLedgers(ctx context.Context, env, tenantID string) ([]model.CostQuotaLedger, error) {
	if s.repo == nil {
		return nil, errors.New("quota repository is not configured")
	}
	return s.repo.ListByTenant(ctx, env, tenantID, 0)
}

// EnforceAction records throttle/degrade/disable decisions.
func (s *Service) EnforceAction(ctx context.Context, env string, input EnforcementInput) (*model.CostQuotaLedger, error) {
	if s.repo == nil {
		return nil, errors.New("quota repository is not configured")
	}
	tenant := strings.TrimSpace(input.TenantID)
	if tenant == "" {
		return nil, errors.New("tenant_id required")
	}
	ledger, err := s.ensureLedgerExists(ctx, env, tenant, input.ProviderID, "monthly")
	if err != nil {
		return nil, err
	}
	enforcement := cloneJSONMap(ledger.EnforcementState)
	updated := s.clock().UTC().Format(time.RFC3339)
	action := strings.ToLower(strings.TrimSpace(input.Action))
	enforcement["action"] = action
	enforcement["reason"] = strings.TrimSpace(input.Reason)
	enforcement["ticketId"] = strings.TrimSpace(input.TicketID)
	enforcement["requestedBy"] = strings.TrimSpace(input.RequestedBy)
	enforcement["updatedAt"] = updated
	event := map[string]any{
		"action":      action,
		"reason":      enforcement["reason"],
		"ticketId":    enforcement["ticketId"],
		"requestedBy": enforcement["requestedBy"],
		"timestamp":   updated,
	}
	enforcement = appendHistoryEvent(enforcement, event)

	if err := s.repo.UpdateUsage(ctx, ledger.UUID, ledger.UsageActual, nil, enforcement, nil); err != nil {
		return nil, err
	}
	ledger, err = s.repo.GetByUUID(ctx, ledger.UUID)
	if err != nil {
		return nil, err
	}
	ledger.EnforcementState = enforcement
	s.cacheSnapshot(ctx, ledger)
	s.emitAudit(ctx, "cost_quota.enforcement", ledger, map[string]any{
		"action": enforcement["action"],
		"reason": enforcement["reason"],
	})
	return ledger, nil
}

func (s *Service) ensureLedgerExists(ctx context.Context, env, tenantID string, providerID *uuid.UUID, period string) (*model.CostQuotaLedger, error) {
	ledger, err := s.repo.FindScope(ctx, env, tenantID, providerID, period)
	if err != nil {
		return nil, err
	}
	if ledger != nil {
		return ledger, nil
	}
	input := LedgerInput{
		TenantID:       tenantID,
		ProviderID:     providerID,
		BudgetPeriod:   period,
		QuotaLimit:     0,
		DashboardScope: "tenant",
	}
	return s.EnsureLedger(ctx, env, input)
}

// Snapshot returns cached ledger state if present.
func (s *Service) Snapshot(ctx context.Context, env, tenantID, providerID string) (*model.CostQuotaLedger, error) {
	if cached := s.fetchSnapshot(ctx, env, tenantID, providerID); cached != nil {
		return cached, nil
	}
	return nil, nil
}

func (s *Service) sealMetadata(ctx context.Context, env string, data map[string]string) (datatypes.JSONMap, error) {
	if len(data) == 0 || s.keys == nil {
		return nil, nil
	}
	raw := datatypes.JSONMap{}
	keys := make([]string, 0, len(data))
	for k, v := range data {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		raw[k] = v
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil, nil
	}
	return s.keys.SealSensitive(ctx, env, nil, raw, keys...)
}

func (s *Service) cacheSnapshot(ctx context.Context, ledger *model.CostQuotaLedger) {
	if s.cache == nil || ledger == nil {
		return
	}
	payload, err := json.Marshal(ledger)
	if err != nil {
		return
	}
	key := fmt.Sprintf(quotaCacheKey, ledger.Env, ledger.TenantID, providerKey(ledger.ProviderProfileID))
	if err := s.cache.Set(ctx, key, payload, 10*time.Minute); err != nil {
		logger.WarnF(ctx, "[cost_quota] cache snapshot failed: %v", err)
	}
}

func (s *Service) fetchSnapshot(ctx context.Context, env, tenantID, providerID string) *model.CostQuotaLedger {
	if s.cache == nil {
		return nil
	}
	key := fmt.Sprintf(quotaCacheKey, env, tenantID, providerID)
	raw, err := s.cache.Get(ctx, key)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var ledger model.CostQuotaLedger
	if err := json.Unmarshal(raw, &ledger); err != nil {
		_ = s.cache.Delete(ctx, key)
		return nil
	}
	return &ledger
}

func (s *Service) emitAudit(ctx context.Context, op string, ledger *model.CostQuotaLedger, meta map[string]any) {
	if s.audit == nil || ledger == nil {
		return
	}
	if meta == nil {
		meta = map[string]any{}
	}
	meta["tenant_id"] = ledger.TenantID
	meta["budget_period"] = ledger.BudgetPeriod
	payload, _ := json.Marshal(meta)
	_ = s.audit.Emit(ctx, &dbmaudit.AuditEvent{
		Source:       "cost_quota.service",
		Operation:    op,
		ResourceType: "agent.cost_quota_ledger",
		ResourceID:   ledger.UUID.String(),
		Outcome:      "SUCCESS",
		Severity:     "INFO",
		Meta:         datatypes.JSON(payload),
		OccurredAt:   s.clock(),
	})
}

func providerKey(id *uuid.UUID) string {
	if id == nil {
		return "tenant"
	}
	return id.String()
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func appendHistoryEvent(state datatypes.JSONMap, event map[string]any) datatypes.JSONMap {
	if state == nil {
		state = datatypes.JSONMap{}
	}
	history := historyRecords(state)
	history = append([]map[string]any{event}, history...)
	if len(history) > maxEnforcementHistory {
		history = history[:maxEnforcementHistory]
	}
	next := make([]any, len(history))
	for i, item := range history {
		next[i] = item
	}
	state["history"] = next
	return state
}

func historyRecords(state datatypes.JSONMap) []map[string]any {
	if state == nil {
		return nil
	}
	raw, ok := state["history"]
	if !ok {
		return nil
	}
	switch arr := raw.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), arr...)
	case []any:
		items := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				items = append(items, m)
			}
		}
		return items
	default:
		return nil
	}
}

func cloneJSONMap(src datatypes.JSONMap) datatypes.JSONMap {
	if src == nil {
		return datatypes.JSONMap{}
	}
	out := datatypes.JSONMap{}
	for k, v := range src {
		out[k] = v
	}
	return out
}
