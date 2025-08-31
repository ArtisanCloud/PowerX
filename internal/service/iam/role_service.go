package iam

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/domain/model"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"strings"

	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

type RoleService struct {
	db   *gorm.DB
	repo *iamrepo.RoleRepository
}

func NewRoleService(db *gorm.DB) *RoleService {
	return &RoleService{db: db, repo: iamrepo.NewRoleRepository(db)}
}

type ListOpt struct {
	TenantID *uint64
	Scope    string
	Keyword  string
	Builtin  *bool
	Page     int
	Size     int
	Sort     string
}

// Create 直接用 GORM 实体；对必要字段做最小校验与修整
// Create：root 可创建 system/tenant；非 root 仅能在自己租户创建 tenant
func (s *RoleService) Create(ctx context.Context, in *dbm.Role) (*dbm.Role, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Scope = strings.ToLower(strings.TrimSpace(in.Scope))

	isRoot := false
	if c := reqctx.GetClaims(ctx); c != nil && c.IsRoot {
		isRoot = true
	}
	ctxTenant := reqctx.GetTenantID(ctx) // 非 root 时用于约束

	switch iam.RoleScope(in.Scope) {
	case iam.RoleScopeTenant:
		// 非 root：只能在自己的租户创建；tenant_id 为空则用上下文补齐
		if !isRoot {
			if in.TenantID == 0 {
				in.TenantID = ctxTenant
			}
			if in.TenantID == 0 || in.TenantID != ctxTenant {
				return nil, errors.New("forbidden: can only create roles in your tenant")
			}
		} else {
			// root：要求 tenant_id>0
			if in.TenantID == 0 {
				return nil, errors.New("tenant role requires tenant_id (>0)")
			}
		}
	case iam.RoleScopeSystem:
		// 非 root 禁止创建 system；root 强制 tenant_id=0
		if !isRoot {
			return nil, errors.New("forbidden: creating system-scope roles requires root")
		}
		in.TenantID = 0 // <-- 修正你原来写成 1 的问题
	default:
		return nil, errors.New("invalid scope (system|tenant)")
	}

	if _, err := s.repo.Create(ctx, in); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errors.New("role code already exists in scope")
		}
		return nil, err
	}
	return in, nil
}

func (s *RoleService) CreateWithPerms(
	ctx context.Context,
	in *dbm.Role,
	permIDs []uint64,
) (*dbm.Role, *SetIDsResult, error) {
	// 1) 先创建角色（沿用 Create 的校验与租户/Root限制）
	out, err := s.Create(ctx, in)
	if err != nil {
		return nil, nil, err
	}

	// 2) 不带 perm_ids 就直接返回
	if len(permIDs) == 0 {
		return out, nil, nil
	}

	// 3) 复用 RBACService 的 SetPermissionIDs（从 ctx 自动取 Root/Tenant）
	rbac := NewRBACService(s.db)
	res, err := rbac.SetPermissionIDs(ctx, out.ID, permIDs)
	if err != nil {
		// 如果你想要“原子性”，这里可按需做补偿删除角色或改为事务
		return out, nil, err
	}

	return out, &res, nil
}

// Update：root 直接放行；非 root 必须匹配本租户（仅改 name/description）
func (s *RoleService) Update(ctx context.Context, id uint64, _ *uint64, patch *dbm.Role) error {
	cur, err := s.repo.GetFirst(ctx, map[string]any{"id": id})
	if err != nil {
		return err
	}
	if cur == nil {
		return gorm.ErrRecordNotFound
	}

	isRoot := false
	if c := reqctx.GetClaims(ctx); c != nil && c.IsRoot {
		isRoot = true
	}
	ctxTenant := reqctx.GetTenantID(ctx)

	if !isRoot {
		switch strings.ToLower(cur.Scope) {
		case "tenant":
			if cur.TenantID == 0 || ctxTenant == 0 || cur.TenantID != ctxTenant {
				return errors.New("tenant scope mismatch")
			}
		case "system":
			// 非 root 不允许改 system
			return errors.New("forbidden: updating system-scope roles requires root")
		default:
			return errors.New("invalid scope")
		}
	}
	updates := map[string]any{
		"name":        strings.TrimSpace(patch.Name),
		"description": strings.TrimSpace(patch.Description),
	}
	_, err = s.repo.UpdateByID(ctx, id, updates, nil)
	return err
}

// Delete：root 放行（仍禁止删除 builtin）；非 root 仅能删本租户 tenant 角色
func (s *RoleService) Delete(ctx context.Context, id uint64, _ *uint64) error {
	cur, err := s.repo.GetFirst(ctx, map[string]any{"id": id})
	if err != nil {
		return err
	}
	if cur == nil {
		return gorm.ErrRecordNotFound
	}
	if cur.Builtin {
		return errors.New("builtin role cannot be deleted")
	}

	isRoot := false
	if c := reqctx.GetClaims(ctx); c != nil && c.IsRoot {
		isRoot = true
	}
	ctxTenant := reqctx.GetTenantID(ctx)

	if !isRoot {
		if strings.ToLower(cur.Scope) == "system" {
			return errors.New("forbidden: deleting system-scope roles requires root")
		}
		if cur.TenantID == 0 || ctxTenant == 0 || cur.TenantID != ctxTenant {
			return errors.New("tenant scope mismatch")
		}
	}

	return s.repo.SafeDelete(ctx, &cur.TenantID, id)
}

// Get：root 可看任意；非 root 只能看自己租户（system 不可见）
func (s *RoleService) Get(ctx context.Context, id uint64, tid uint64) (*dbm.Role, error) {
	m, err := s.repo.GetFirst(ctx, map[string]any{"id": id})
	if err != nil || m == nil {
		return m, err
	}

	isRoot := false
	if c := reqctx.GetClaims(ctx); c != nil && c.IsRoot {
		isRoot = true
	}
	if isRoot {
		return m, nil
	}

	if strings.ToLower(m.Scope) == "tenant" {
		if m.TenantID == 0 || tid == 0 || m.TenantID != tid {
			return nil, gorm.ErrRecordNotFound
		}
		return m, nil
	}
	// 非 root 看不到 system
	return nil, gorm.ErrRecordNotFound
}

// List：root 可全局筛选；非 root 强制 scope=tenant 且 tenant_id=ctxTenant
func (s *RoleService) List(ctx context.Context, opt ListOpt) (model.Page[dbm.Role], error) {
	isRoot := false
	if c := reqctx.GetClaims(ctx); c != nil && c.IsRoot {
		isRoot = true
	}
	ctxTenant := reqctx.GetTenantID(ctx)

	if !isRoot {
		// 非 root：确定一个有效的 tenant_id（优先上下文，没有则用传入的 opt.TenantID）
		var effTenant uint64
		if ctxTenant > 0 {
			effTenant = ctxTenant
		} else if opt.TenantID != nil && *opt.TenantID > 0 {
			effTenant = *opt.TenantID
		}
		if effTenant == 0 {
			return model.Page[dbm.Role]{}, errors.New("tenant context required")
		}
		// 强制限定到当前租户 + tenant 作用域
		opt.Scope = "tenant"
		opt.TenantID = &effTenant
	}

	items, total, err := s.repo.FindPage(ctx, iamrepo.ListRoleOption{
		TenantID: opt.TenantID,
		Scope:    opt.Scope,
		Keyword:  opt.Keyword,
		Builtin:  opt.Builtin,
		Page:     opt.Page,
		PageSize: opt.Size,
		Sort:     opt.Sort,
	})
	if err != nil {
		return model.Page[dbm.Role]{}, err
	}
	return model.Page[dbm.Role]{List: items, Total: total, PageIndex: opt.Page, PageSize: opt.Size}, nil
}
