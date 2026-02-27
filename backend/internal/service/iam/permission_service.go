// internal/service/iam/permission_service.go
package iam

import (
	"context"
	"encoding/json"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"strings"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"gorm.io/gorm"
)

type PermissionRegisterItem struct {
	Plugin, Resource, Action, Effect, Description string
	Label, Module, Type, APIEndpoint, HTTPMethod  string
}

type ActorContext struct {
	IsRoot     bool
	TenantUUID string
}

type SetIDsResult struct {
	Added             []uint64 `json:"added"`
	Removed           []uint64 `json:"removed"`
	Now               []uint64 `json:"now"`
	SkippedDeprecated []uint64 `json:"skipped_deprecated"`
}

type PermissionListOpt struct {
	Plugin, Resource, Module, Type, Keyword, Sort string
	Page, Size                                    int
}

type PermissionPage struct {
	List      []PermissionView
	Total     int64
	PageIndex int
	PageSize  int
}

type PermissionView struct {
	ID                                            uint64
	Plugin, Resource, Action, Effect, Description string
	Label, Module, Type, APIEndpoint, HTTPMethod  string
}

type PermissionService struct {
	db *gorm.DB

	roles    *repo.RoleRepository
	perms    *repo.PermissionRepository
	rolePerm *repo.RolePermissionRepository
}

func NewPermissionService(db *gorm.DB) *PermissionService {
	return &PermissionService{
		db:       db,
		roles:    repo.NewRoleRepository(db),
		perms:    repo.NewPermissionRepository(db),
		rolePerm: repo.NewRolePermissionRepository(db),
	}
}

// RegisterPermissions —— 直接接收并写入 GORM 实体（不自定义 DTO）
func (s *PermissionService) RegisterPermissions(ctx context.Context, rows []dbm.Permission) error {
	// 兜底：Effect/Status 清洗
	for i := range rows {
		if strings.TrimSpace(rows[i].Effect) == "" {
			rows[i].Effect = "allow"
		}
		if rows[i].Status == "" {
			rows[i].Status = dbm.PermissionStatusActive
		}
		// 不改 Meta：上层已经按 datatypes.JSON 赋值
	}
	return s.perms.UpsertBatch(ctx, rows)
}

// ListPermissions —— 返回原始 GORM 实体列表与总数（不定义额外 View）
func (s *PermissionService) ListPermissions(ctx context.Context, filter map[string]string, page, size int, sort string) ([]dbm.Permission, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 200 {
		size = 50
	}
	rows, total, err := s.perms.List(ctx, filter, (page-1)*size, size, sort)
	return rows, total, err
}

// ListCatalog —— 聚合为 module->type->[]Permission（仍用 GORM 实体；Meta 在 handler 展开即可）
func (s *PermissionService) ListCatalog(ctx context.Context) (map[string]map[string][]dbm.Permission, error) {
	rows, _, err := s.perms.List(ctx, map[string]string{
		"status": string(dbm.PermissionStatusActive),
	}, 0, 10000, "module ASC, resource ASC, action ASC")
	if err != nil {
		return nil, err
	}
	out := map[string]map[string][]dbm.Permission{}
	for _, p := range rows {
		// 尝试从 Meta 读取 module/type
		var m map[string]any
		_ = json.Unmarshal(p.Meta, &m)
		mod := strings.TrimSpace(utils.ToStr(m["module"]))
		if mod == "" {
			mod = p.Module
		}
		tp := strings.TrimSpace(utils.ToStr(m["type"]))
		if tp == "" {
			tp = "action"
		}
		if _, ok := out[mod]; !ok {
			out[mod] = map[string][]dbm.Permission{}
		}
		out[mod][tp] = append(out[mod][tp], p)
	}
	return out, nil
}

// SyncPermissions —— 用 GORM 实体同步；dryRun=true 仅返回 diff，不落库
func (s *PermissionService) SyncPermissions(
	ctx context.Context,
	source string,
	introduced string,
	rows []dbm.Permission,
	dryRun bool,
) (repo.SyncResult, error) {
	// 兜底清洗
	for i := range rows {
		if strings.TrimSpace(rows[i].Effect) == "" {
			rows[i].Effect = "allow"
		}
		rows[i].Source = source
		if strings.TrimSpace(rows[i].Introduced) == "" {
			rows[i].Introduced = introduced
		}
		if rows[i].Status == "" {
			rows[i].Status = dbm.PermissionStatusActive
		}
	}

	// 调用仓储同步
	res, err := s.perms.Sync(ctx, source, introduced, rows, dryRun)
	if err != nil || dryRun {
		return res, err
	}

	// 同步成功且非 dryRun：自动把全部 active 权限授予 root 角色（若存在）
	go func() {
		ctx2 := context.Background()
		rr := repo.NewRoleRepository(s.db)
		rpr := repo.NewRolePermissionRepository(s.db)
		pr := repo.NewPermissionRepository(s.db)

		root, _ := rr.GetFirst(ctx2, "scope = ? AND code = ?", "system", "root")
		if root == nil {
			return
		}

		ids, _ := pr.ListIDsByCondition(ctx2, map[string]any{
			"status": dbm.PermissionStatusActive,
		})
		if len(ids) == 0 {
			return
		}

		_ = s.db.WithContext(ctx2).Transaction(func(tx *gorm.DB) error {
			return rpr.GrantByIDsTx(tx, root.ID, ids)
		})
	}()

	return res, nil
}

func (s *PermissionService) UpdatePermissionFields(ctx context.Context, id uint64, description *string, status *string, deprecatedAt *int64) error {
	updates := map[string]any{}
	if description != nil {
		updates["description"] = *description
	}
	if status != nil {
		updates["status"] = *status
		// deprecated_at: 仅当字段存在时更新
		if status != nil {
			if deprecatedAt != nil {
				updates["deprecated_at"] = *deprecatedAt
			} else {
				// 置空
				updates["deprecated_at"] = gorm.Expr("NULL")
			}
		}
	}
	if len(updates) == 0 {
		return nil
	}
	_, err := s.perms.UpdateByID(ctx, id, updates, nil)
	return err
}
