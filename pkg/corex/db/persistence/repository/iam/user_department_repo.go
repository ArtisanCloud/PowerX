// internal/infra/persistence/iam/repo_user_department.go
package iam

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type UserDepartmentRepository struct {
	*repository.BaseRepository[dbm.UserDepartment]
	db *gorm.DB
}

func NewUserDepartmentRepository(db *gorm.DB) *UserDepartmentRepository {
	return &UserDepartmentRepository{
		BaseRepository: repository.NewBaseRepository[dbm.UserDepartment](db),
		db:             db,
	}
}

func (r *UserDepartmentRepository) Bind(ctx context.Context, tenantID, userID uint64, deptIDs ...uint64) error {
	if len(deptIDs) == 0 {
		return nil
	}
	rows := make([]dbm.UserDepartment, 0, len(deptIDs))
	for _, did := range deptIDs {
		rows = append(rows, dbm.UserDepartment{UserID: userID, DepartmentID: did, TenantID: tenantID})
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func (r *UserDepartmentRepository) ListUserDepartments(ctx context.Context, tenantID, userID uint64) ([]dbm.Department, error) {
	var list []dbm.Department
	err := r.db.WithContext(ctx).
		Table((&dbm.Department{}).GetTableName(true)+" d").
		Select("d.*").
		Joins("JOIN "+(&dbm.UserDepartment{}).GetTableName(true)+" ud ON ud.department_id = d.id AND ud.tenant_id = ?", tenantID).
		Where("ud.user_id = ?", userID).
		Find(&list).Error
	return list, err
}
