// pkg/corex/db/persistence/repository/iam/role_binding_repo.go
package iam

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type RoleBindingRepository struct {
	*repository.BaseRepository[dbm.RoleBinding]
	db *gorm.DB
}

func NewRoleBindingRepository(db *gorm.DB) *RoleBindingRepository {
	return &RoleBindingRepository{
		BaseRepository: repository.NewBaseRepository[dbm.RoleBinding](db),
		db:             db,
	}
}

func (r *RoleBindingRepository) Create(ctx context.Context, rb *dbm.RoleBinding) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(rb).Error
}

func (r *RoleBindingRepository) Delete(ctx context.Context, tenantID, id uint64) error {
	return r.db.WithContext(ctx).Where("tenant_id=? AND id=?", tenantID, id).Delete(&dbm.RoleBinding{}).Error
}

func (r *RoleBindingRepository) ListBySubject(ctx context.Context, tenantID uint64, st dbm.SubjectType, sid uint64) ([]dbm.RoleBinding, error) {
	var out []dbm.RoleBinding
	err := r.db.WithContext(ctx).Where("tenant_id=? AND subject_type=? AND subject_id=?", tenantID, st, sid).Find(&out).Error
	return out, err
}

// （用于授权器预热）拉取某成员“有效”的所有绑定：直绑 + 通过 Assignment 拉来的绑定
// RoleBindingRepository.ListEffectiveForMember
func (r *RoleBindingRepository) ListEffectiveForMember(ctx context.Context, tenantID, memberID uint64) ([]dbm.RoleBinding, error) {
	tRB := (&dbm.RoleBinding{}).GetTableName(true)
	tMA := (&dbm.MemberAssignment{}).GetTableName(true)

	var out []dbm.RoleBinding
	err := r.db.WithContext(ctx).Raw(`
		SELECT rb.* FROM `+tRB+` rb
		WHERE rb.tenant_id = ? AND rb.subject_type = ? AND rb.subject_id = ?
		UNION ALL
		SELECT rb.* FROM `+tRB+` rb
		JOIN `+tMA+` ma
		  ON ma.tenant_id = rb.tenant_id
		 AND rb.subject_type = ma.dim_type
		 AND rb.subject_id = ma.dim_id
		WHERE rb.tenant_id = ? AND ma.member_id = ?`,
		tenantID, dbm.SubMember, memberID,
		tenantID, memberID,
	).Scan(&out).Error
	return out, err
}
