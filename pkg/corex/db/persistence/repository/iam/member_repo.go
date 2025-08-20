package iam

import (
	"context"
	"strings"

	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type MemberRepository struct {
	*repository.BaseRepository[dbm.Member]
	db *gorm.DB
}

func NewMemberRepository(db *gorm.DB) *MemberRepository {
	return &MemberRepository{
		BaseRepository: repository.NewBaseRepository[dbm.Member](db),
		db:             db,
	}
}

func (r *MemberRepository) FindByID(ctx context.Context, id uint64) (*dbm.Member, error) {
	var u dbm.Member
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *MemberRepository) FindByUsername(ctx context.Context, tenantID uint64, username string) (*dbm.Member, error) {
	var u dbm.Member
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND username = ?", tenantID, username).
		First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *MemberRepository) FindByEmail(ctx context.Context, tenantID uint64, email string) (*dbm.Member, error) {
	var u dbm.Member
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND email = ?", tenantID, email).
		First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *MemberRepository) FindByPhone(ctx context.Context, tenantID uint64, phone string) (*dbm.Member, error) {
	var u dbm.Member
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND phone = ?", tenantID, phone).
		First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindByTenantAndUser 按租户 + 全局用户ID 查成员
func (r *MemberRepository) FindByTenantAndUser(ctx context.Context, tenantID, userID uint64) (*dbm.Member, error) {
	var m dbm.Member
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// （可选）FindByTenantAndUsername 按租户 + 用户名查成员（用户名统一小写）
func (r *MemberRepository) FindByTenantAndUsername(ctx context.Context, tenantID uint64, username string) (*dbm.Member, error) {
	var m dbm.Member
	username = strings.ToLower(strings.TrimSpace(username))
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND username = ?", tenantID, username).
		First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// List 支持 keyword/status/部门过滤（部门可选：通过 user->dept 关系表或 user.meta 里的 dept_id）
// 这里给出通用 keyword/status，部门过滤你按自己的关联表补充
type MemberListFilter struct {
	TenantID uint64
	Keyword  string
	Status   *int16
	Page     int
	Size     int
}
