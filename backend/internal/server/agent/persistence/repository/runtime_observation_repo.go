package repository

import (
	"context"
	"fmt"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RuntimeObservationRepository is deliberately persistence-only. Authorization
// and descriptor validation belong to RuntimeSnapshotService before a row is
// written.
type RuntimeObservationRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentRunSnapshot]
	db *gorm.DB
}

func NewRuntimeObservationRepository(db *gorm.DB) *RuntimeObservationRepository {
	return &RuntimeObservationRepository{BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentRunSnapshot](db), db: db}
}

func (r *RuntimeObservationRepository) CreateSnapshot(ctx context.Context, row *dbmodel.AgentRunSnapshot) error {
	if r == nil || r.db == nil || row == nil {
		return fmt.Errorf("runtime snapshot repository is not configured")
	}
	if strings.TrimSpace(row.Env) == "" || strings.TrimSpace(row.TenantUUID) == "" || row.RunUUID == uuid.Nil || row.AgentUUID == uuid.Nil || strings.TrimSpace(row.SubjectUUID) == "" || strings.TrimSpace(row.PolicyVersion) == "" || strings.TrimSpace(row.AuthorizationFingerprint) == "" {
		return fmt.Errorf("runtime snapshot identity is required")
	}
	if len(row.Descriptors) == 0 {
		return fmt.Errorf("runtime snapshot descriptors are required")
	}
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *RuntimeObservationRepository) GetSnapshot(ctx context.Context, env, tenantUUID string, runUUID uuid.UUID) (*dbmodel.AgentRunSnapshot, error) {
	if r == nil || r.db == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || runUUID == uuid.Nil {
		return nil, fmt.Errorf("runtime snapshot scope is required")
	}
	var row dbmodel.AgentRunSnapshot
	if err := r.db.WithContext(ctx).Where("env = ? AND tenant_uuid = ? AND run_uuid = ?", strings.TrimSpace(env), strings.TrimSpace(tenantUUID), runUUID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *RuntimeObservationRepository) CreateObservation(ctx context.Context, row *dbmodel.AgentRunObservation) error {
	if r == nil || r.db == nil || row == nil {
		return fmt.Errorf("runtime observation repository is not configured")
	}
	if strings.TrimSpace(row.Env) == "" || strings.TrimSpace(row.TenantUUID) == "" || row.RunUUID == uuid.Nil || row.SnapshotUUID == uuid.Nil || row.ResourceUUID == uuid.Nil || strings.TrimSpace(row.Purpose) == "" || strings.TrimSpace(row.Digest) == "" {
		return fmt.Errorf("runtime observation identity is required")
	}
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *RuntimeObservationRepository) ListObservations(ctx context.Context, env, tenantUUID string, runUUID uuid.UUID) ([]dbmodel.AgentRunObservation, error) {
	if r == nil || r.db == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || runUUID == uuid.Nil {
		return nil, fmt.Errorf("runtime observation scope is required")
	}
	var rows []dbmodel.AgentRunObservation
	err := r.db.WithContext(ctx).Where("env = ? AND tenant_uuid = ? AND run_uuid = ?", strings.TrimSpace(env), strings.TrimSpace(tenantUUID), runUUID).Order("created_at ASC").Find(&rows).Error
	return rows, err
}
