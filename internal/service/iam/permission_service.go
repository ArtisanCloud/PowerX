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
	db   *gorm.DB
	perm *repo.PermissionRepository
}

func NewPermissionService(db *gorm.DB) *PermissionService {
	return &PermissionService{
		db:   db,
		perm: repo.NewPermissionRepository(db),
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
	return s.perm.UpsertBatch(ctx, rows)
}

// ListPermissions —— 返回原始 GORM 实体列表与总数（不定义额外 View）
func (s *PermissionService) ListPermissions(ctx context.Context, filter map[string]string, page, size int, sort string) ([]dbm.Permission, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 200 {
		size = 50
	}
	rows, total, err := s.perm.List(ctx, filter, (page-1)*size, size, sort)
	return rows, total, err
}

// ListCatalog —— 聚合为 module->type->[]Permission（仍用 GORM 实体；Meta 在 handler 展开即可）
func (s *PermissionService) ListCatalog(ctx context.Context) (map[string]map[string][]dbm.Permission, error) {
	rows, _, err := s.perm.List(ctx, map[string]string{
		"status": string(dbm.PermissionStatusActive),
	}, 0, 10000, "plugin ASC, resource ASC, action ASC")
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
			mod = p.Plugin
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

// SyncPermissions —— 直接用 GORM 实体进行同步（不自定义请求结构）
func (s *PermissionService) SyncPermissions(ctx context.Context, source, introduced string, rows []dbm.Permission, dryRun bool) (repo.SyncResult, error) {
	// 兜底清洗
	for i := range rows {
		if strings.TrimSpace(rows[i].Effect) == "" {
			rows[i].Effect = "allow"
		}
		rows[i].Source = source
		if strings.TrimSpace(rows[i].Introduced) == "" {
			rows[i].Introduced = introduced
		}
		rows[i].Status = dbm.PermissionStatusActive
	}
	return s.perm.Sync(ctx, source, introduced, rows, dryRun)
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
	_, err := s.perm.UpdateByID(ctx, id, updates, nil)
	return err
}
