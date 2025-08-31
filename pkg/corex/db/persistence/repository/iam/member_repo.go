package iam

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
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

func (r *MemberRepository) FindByUserName(ctx context.Context, tenantID uint64, username string) (*dbm.Member, error) {
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

// 已有：按租户+用户名查
func (r *MemberRepository) FindByTenantAndUserName(ctx context.Context, tenantID uint64, username string) (*dbm.Member, error) {
	var m dbm.Member
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND username = ?", tenantID, username).
		First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// 已有：按租户+User 查
func (r *MemberRepository) FindByTenantAndUser(ctx context.Context, tenantID uint64, userID uint64) (*dbm.Member, error) {
	var m dbm.Member
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// ✅ 新增：按 UserID 列出该用户在所有租户的成员记录
func (r *MemberRepository) ListByUserID(ctx context.Context, userID uint64) ([]*dbm.Member, error) {
	var list []*dbm.Member
	if userID == 0 {
		return list, nil
	}
	err := r.DB.WithContext(ctx).
		Table(model.TableIAMMember).
		Where("user_id = ?", userID).
		Find(&list).Error
	return list, err
}

// （可选）如果你想带状态过滤：
func (r *MemberRepository) ListByUserIDAndStatus(ctx context.Context, userID uint64, status *int16) ([]dbm.Member, error) {
	var list []dbm.Member
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if status != nil {
		q = q.Where("status = ?", *status)
	}
	if err := q.Order("id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
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
