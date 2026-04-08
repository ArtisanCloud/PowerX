package ops

import (
	"context"
	"errors"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ApprovalPolicyProfileRepository struct {
	*repository.BaseRepository[modelops.ApprovalPolicyProfile]
	db *gorm.DB
}

func NewApprovalPolicyProfileRepository(db *gorm.DB) *ApprovalPolicyProfileRepository {
	return &ApprovalPolicyProfileRepository{
		BaseRepository: repository.NewBaseRepository[modelops.ApprovalPolicyProfile](db),
		db:             db,
	}
}

func (r *ApprovalPolicyProfileRepository) FindByEnvironment(ctx context.Context, environment string) (*modelops.ApprovalPolicyProfile, error) {
	var row modelops.ApprovalPolicyProfile
	if err := r.db.WithContext(ctx).Where("environment = ?", environment).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *ApprovalPolicyProfileRepository) UpsertByEnvironment(ctx context.Context, row *modelops.ApprovalPolicyProfile) (*modelops.ApprovalPolicyProfile, error) {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "environment"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"approval_mode", "updated_by", "updated_at",
			}),
		}).
		Create(row).Error
	if err != nil {
		return nil, err
	}
	return row, nil
}
