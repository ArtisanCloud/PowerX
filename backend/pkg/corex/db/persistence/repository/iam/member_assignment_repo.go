// pkg/corex/db/persistence/repository/iam/member_assignment_repo.go
package iam

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type MemberAssignmentRepository struct {
	*repository.BaseRepository[dbm.MemberAssignment]
	db *gorm.DB
}

func NewMemberAssignmentRepository(db *gorm.DB) *MemberAssignmentRepository {
	return &MemberAssignmentRepository{
		BaseRepository: repository.NewBaseRepository[dbm.MemberAssignment](db),
		db:             db,
	}
}

func (r *MemberAssignmentRepository) Bind(ctx context.Context, tenantUUID string, memberID uint64, dim dbm.AssignmentDim, dimIDs ...uint64) error {
	if len(dimIDs) == 0 {
		return nil
	}
	rows := make([]dbm.MemberAssignment, 0, len(dimIDs))
	for _, id := range dimIDs {
		rows = append(rows, dbm.MemberAssignment{
			TenantUUID: tenantUUID, MemberID: memberID, DimType: dim, DimID: id,
		})
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func (r *MemberAssignmentRepository) ListByMember(ctx context.Context, tenantUUID string, memberID uint64) ([]dbm.MemberAssignment, error) {
	var out []dbm.MemberAssignment
	err := r.db.WithContext(ctx).Where("tenant_uuid=? AND member_id=?", tenantUUID, memberID).Find(&out).Error
	return out, err
}

func (r *MemberAssignmentRepository) ListMembersByDim(ctx context.Context, tenantUUID string, dim dbm.AssignmentDim, dimID uint64) ([]dbm.MemberAssignment, error) {
	var out []dbm.MemberAssignment
	err := r.db.WithContext(ctx).Where("tenant_uuid=? AND dim_type=? AND dim_id=?", tenantUUID, dim, dimID).Find(&out).Error
	return out, err
}
