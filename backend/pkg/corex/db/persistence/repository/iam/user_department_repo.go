// internal/infra/persistence/iam/repo_member_department.go
package iam

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type MemberDepartmentRepository struct {
	*repository.BaseRepository[dbm.MemberDepartment]
	db *gorm.DB
}

func NewMemberDepartmentRepository(db *gorm.DB) *MemberDepartmentRepository {
	return &MemberDepartmentRepository{
		BaseRepository: repository.NewBaseRepository[dbm.MemberDepartment](db),
		db:             db,
	}
}

func (r *MemberDepartmentRepository) Bind(ctx context.Context, tenantUUID string, memberID uint64, deptIDs ...uint64) error {
	if len(deptIDs) == 0 {
		return nil
	}
	rows := make([]dbm.MemberDepartment, 0, len(deptIDs))
	for _, did := range deptIDs {
		rows = append(rows, dbm.MemberDepartment{
			MemberID:     memberID,
			DepartmentID: did,
			TenantUUID:   tenantUUID,
		})
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func (r *MemberDepartmentRepository) ListMemberDepartments(ctx context.Context, tenantUUID string, memberID uint64) ([]dbm.Department, error) {
	var list []dbm.Department
	err := r.db.WithContext(ctx).
		Table((&dbm.Department{}).GetTableName(true)+" d").
		Select("d.*").
		Joins("JOIN "+(&dbm.MemberDepartment{}).GetTableName(true)+" ud ON ud.department_id = d.id AND ud.tenant_uuid = ?", tenantUUID).
		Where("ud.member_id = ?", memberID).
		Find(&list).Error
	return list, err
}
