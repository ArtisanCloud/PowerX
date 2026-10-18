package agent

import (
	"context"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EvidenceSnapshotRepository struct {
	*base.BaseRepository[model.EvidenceSnapshot]
	db *gorm.DB
}

func NewEvidenceSnapshotRepository(db *gorm.DB) *EvidenceSnapshotRepository {
	return &EvidenceSnapshotRepository{BaseRepository: base.NewBaseRepository[model.EvidenceSnapshot](db), db: db}
}
func (r *EvidenceSnapshotRepository) Ready(ctx context.Context) error {
	if r == nil || r.db == nil || !r.db.WithContext(ctx).Migrator().HasTable(&model.EvidenceSnapshot{}) {
		return gorm.ErrInvalidDB
	}
	return nil
}
func (r *EvidenceSnapshotRepository) SaveSnapshot(ctx context.Context, row *model.EvidenceSnapshot) error {
	return r.db.WithContext(ctx).Create(row).Error
}
func (r *EvidenceSnapshotRepository) ReadSnapshot(ctx context.Context, tenant, subject, id uuid.UUID) (*model.EvidenceSnapshot, error) {
	var row model.EvidenceSnapshot
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND subject_uuid = ? AND uuid = ? AND status = 1", tenant, subject, id).First(&row).Error
	return &row, err
}
