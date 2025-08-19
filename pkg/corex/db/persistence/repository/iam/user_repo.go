package iam

import (
	"context"

	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type UserRepository struct {
	*repository.BaseRepository[dbm.User]
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{
		BaseRepository: repository.NewBaseRepository[dbm.User](db),
		db:             db,
	}
}

func (r *UserRepository) FindByID(ctx context.Context, id uint64) (*dbm.User, error) {
	var u dbm.User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByUsername(ctx context.Context, tenantID uint64, username string) (*dbm.User, error) {
	var u dbm.User
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND username = ?", tenantID, username).
		First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, tenantID uint64, email string) (*dbm.User, error) {
	var u dbm.User
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND email = ?", tenantID, email).
		First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByPhone(ctx context.Context, tenantID uint64, phone string) (*dbm.User, error) {
	var u dbm.User
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND phone = ?", tenantID, phone).
		First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// List 支持 keyword/status/部门过滤（部门可选：通过 user->dept 关系表或 user.meta 里的 dept_id）
// 这里给出通用 keyword/status，部门过滤你按自己的关联表补充
type UserListFilter struct {
	TenantID uint64
	Keyword  string
	Status   *int16
	Page     int
	Size     int
}

func (r *UserRepository) List(ctx context.Context, f UserListFilter) (list []dbm.User, total int64, err error) {
	q := r.db.WithContext(ctx).Model(&dbm.User{}).Where("tenant_id = ?", f.TenantID)
	if f.Keyword != "" {
		kw := "%" + f.Keyword + "%"
		q = q.Where("(username ILIKE ? OR email ILIKE ? OR phone ILIKE ? OR display_name ILIKE ?)", kw, kw, kw, kw)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if err = q.Count(&total).Error; err != nil {
		return
	}
	if f.Page > 0 && f.Size > 0 {
		q = q.Offset((f.Page - 1) * f.Size).Limit(f.Size)
	}
	err = q.Order("id DESC").Find(&list).Error
	return
}
