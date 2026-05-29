// pkg/corex/db/persistence/repository/iam/user_repo.go
package iam

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"strings"

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

// ---- 全局查找 ----

func (r *UserRepository) FindByID(ctx context.Context, id uint64) (*dbm.User, error) {
	var u dbm.User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*dbm.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u dbm.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByPhone(ctx context.Context, phone string) (*dbm.User, error) {
	phone = strings.TrimSpace(phone)
	var u dbm.User
	if err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ---- 全局列表（不带租户）----
// 说明：租户内列表请用 MemberRepository.List（见下方补充）
type UserListFilter struct {
	TenantUUID string
	Keyword    string
	Status     *int16
	Page       int
	Size       int
	OrderBy    string // 可选：默认 id DESC
}

func (r *UserRepository) List(ctx context.Context, f UserListFilter) (list []dbm.User, total int64, err error) {
	q := r.db.WithContext(ctx).Model(&dbm.User{})

	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		kw = strings.ToLower(kw)
		pattern := "%" + kw + "%"
		// 用 LOWER 兼容 PG/MySQL（避免 ILIKE 的方言差异）
		q = q.Where("(LOWER(email) LIKE ? OR LOWER(phone) LIKE ? OR LOWER(display_name) LIKE ?)",
			pattern, pattern, pattern)
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
	order := "id DESC"
	if f.OrderBy != "" {
		order = f.OrderBy
	}
	err = q.Order(order).Find(&list).Error
	return
}

// 可选：按显示名精确匹配（用于本地 seed 时简单查 root）
func (r *UserRepository) FindByDisplayName(ctx context.Context, name string) (*dbm.User, error) {
	var u dbm.User
	name = strings.TrimSpace(name)
	if err := r.db.WithContext(ctx).Where("display_name = ?", name).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) IsRootUser(ctx context.Context, userID uint64) (bool, error) {
	if userID == 0 {
		return false, nil
	}
	type row struct {
		IsRoot bool `gorm:"column:is_root"`
	}
	var u row
	err := r.DB.WithContext(ctx).
		Table(model.TableIAMUser).
		Select("is_root").
		Where("id = ?", userID).
		Take(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return u.IsRoot, err
}

func (r *UserRepository) UpdateLastTenantUUID(ctx context.Context, userID uint64, tenantUUID string) error {
	if userID == 0 {
		return nil
	}
	tenantUUID = strings.TrimSpace(tenantUUID)
	return r.db.WithContext(ctx).
		Model(&dbm.User{}).
		Where("id = ?", userID).
		Update("last_tenant_uuid", tenantUUID).Error
}
