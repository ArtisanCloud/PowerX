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

type VerificationEvidenceRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentVerificationEvidence]
	db *gorm.DB
}

func NewVerificationEvidenceRepository(db *gorm.DB) *VerificationEvidenceRepository {
	return &VerificationEvidenceRepository{BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentVerificationEvidence](db), db: db}
}

func (r *VerificationEvidenceRepository) Create(ctx context.Context, row *dbmodel.AgentVerificationEvidence) error {
	if r == nil || r.db == nil || row == nil {
		return fmt.Errorf("verification evidence repository is not configured")
	}
	if strings.TrimSpace(row.Env) == "" || strings.TrimSpace(row.TenantUUID) == "" || row.RunUUID == uuid.Nil || row.SnapshotUUID == uuid.Nil || strings.TrimSpace(row.VerdictClass) == "" || strings.TrimSpace(row.ReasonCode) == "" || len(row.TaskRefs) == 0 || len(row.RequiredInputFields) == 0 || len(row.RequiredPermissionCodes) == 0 {
		return fmt.Errorf("verification evidence identity is required")
	}
	return r.db.WithContext(ctx).Create(row).Error
}
