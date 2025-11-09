package agent_model_hub

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// CostQuotaLedgerRepository manages quota + usage ledgers.
type CostQuotaLedgerRepository struct {
	*base.BaseRepository[model.CostQuotaLedger]
	db *gorm.DB
}

func NewCostQuotaLedgerRepository(db *gorm.DB) *CostQuotaLedgerRepository {
	return &CostQuotaLedgerRepository{
		BaseRepository: base.NewBaseRepository[model.CostQuotaLedger](db),
		db:             db,
	}
}

func (r *CostQuotaLedgerRepository) WithDB(db *gorm.DB) *CostQuotaLedgerRepository {
	return NewCostQuotaLedgerRepository(db)
}

// UpsertScope stores or updates a ledger keyed by env+tenant+provider(optional)+period.
func (r *CostQuotaLedgerRepository) UpsertScope(ctx context.Context, ledger *model.CostQuotaLedger) (*model.CostQuotaLedger, error) {
	if ledger == nil {
		return nil, errors.New("ledger is nil")
	}
	existing, err := r.findScope(ctx, ledger.Env, ledger.TenantID, ledger.ProviderProfileID, ledger.BudgetPeriod)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		if err := r.db.WithContext(ctx).Create(ledger).Error; err != nil {
			return nil, err
		}
		return ledger, nil
	}

	update := map[string]any{
		"quota_limit":       ledger.QuotaLimit,
		"dashboard_scope":   ledger.DashboardScope,
		"anomaly_state":     ledger.AnomalyState,
		"enforcement_state": ledger.EnforcementState,
		"sealed_metadata":   ledger.SealedMetadata,
	}
	if err := r.db.WithContext(ctx).
		Model(&model.CostQuotaLedger{}).
		Where("uuid = ?", existing.UUID).
		Updates(update).Error; err != nil {
		return nil, err
	}
	existing.QuotaLimit = ledger.QuotaLimit
	existing.DashboardScope = ledger.DashboardScope
	existing.AnomalyState = ledger.AnomalyState
	existing.EnforcementState = ledger.EnforcementState
	existing.SealedMetadata = ledger.SealedMetadata
	return existing, nil
}

func (r *CostQuotaLedgerRepository) findScope(ctx context.Context, env, tenantID string, providerID *uuid.UUID, period string) (*model.CostQuotaLedger, error) {
	var record model.CostQuotaLedger
	query := r.db.WithContext(ctx).
		Where("env = ? AND tenant_id = ? AND budget_period = ?", env, tenantID, period)
	if providerID == nil {
		query = query.Where("provider_profile_id IS NULL")
	} else {
		query = query.Where("provider_profile_id = ?", providerID)
	}
	err := query.First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// UpdateUsage atomically updates usage/anomaly state by ledger UUID.
func (r *CostQuotaLedgerRepository) UpdateUsage(ctx context.Context, ledgerID uuid.UUID, usage float64, anomaly, enforcement datatypes.JSONMap, lastAnomalyAt *time.Time) error {
	update := map[string]any{
		"usage_actual": usage,
	}
	if anomaly != nil {
		update["anomaly_state"] = anomaly
	}
	if enforcement != nil {
		update["enforcement_state"] = enforcement
	}
	if lastAnomalyAt != nil {
		update["last_anomaly_at"] = *lastAnomalyAt
	}
	return r.db.WithContext(ctx).
		Model(&model.CostQuotaLedger{}).
		Where("uuid = ?", ledgerID).
		Updates(update).Error
}

// ListByTenant returns ledgers for dashboards.
func (r *CostQuotaLedgerRepository) ListByTenant(ctx context.Context, env, tenantID string, limit int) ([]model.CostQuotaLedger, error) {
	query := r.db.WithContext(ctx).
		Where("env = ? AND tenant_id = ?", env, tenantID).
		Order("budget_period DESC, created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var records []model.CostQuotaLedger
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *CostQuotaLedgerRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*model.CostQuotaLedger, error) {
	var record model.CostQuotaLedger
	err := r.db.WithContext(ctx).Where("uuid = ?", id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// UpdateSealedMetadata replaces encrypted enforcement metadata.
func (r *CostQuotaLedgerRepository) UpdateSealedMetadata(ctx context.Context, id uuid.UUID, payload datatypes.JSONMap) error {
	return r.db.WithContext(ctx).
		Model(&model.CostQuotaLedger{}).
		Where("uuid = ?", id).
		Update("sealed_metadata", payload).Error
}
