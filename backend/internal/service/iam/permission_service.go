// internal/service/iam/permission_service.go
package iam

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
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

type PluginPermissionCatalogFilter struct {
	PluginID string
	Module   string
	Type     string
	Status   string
}

type PluginPermissionCatalog struct {
	Plugins []PluginPermissionCatalogPlugin `json:"plugins"`
}

type PluginPermissionCatalogPlugin struct {
	PluginID string                          `json:"plugin_id"`
	Modules  []PluginPermissionCatalogModule `json:"modules"`
}

type PluginPermissionCatalogModule struct {
	Module string                             `json:"module"`
	Types  []PluginPermissionCatalogTypeGroup `json:"types"`
}

type PluginPermissionCatalogTypeGroup struct {
	Type        string                        `json:"type"`
	Permissions []PluginPermissionCatalogItem `json:"permissions"`
}

type PluginPermissionCatalogItem struct {
	ID                     uint64            `json:"id"`
	PermissionCode         string            `json:"permission_code"`
	TitleI18n              map[string]string `json:"title_i18n"`
	DescriptionI18n        map[string]string `json:"description_i18n"`
	RiskLevel              string            `json:"risk_level"`
	DataScope              string            `json:"data_scope,omitempty"`
	BusinessPermissionCode string            `json:"business_permission_code,omitempty"`
	ProtocolBindings       any               `json:"protocol_bindings,omitempty"`
	DefaultRoleGrants      []string          `json:"default_role_grants,omitempty"`
	Status                 string            `json:"status"`
	RegistrationStatus     string            `json:"registration_status"`
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

func (s *PermissionService) ListPluginCatalog(ctx context.Context, filter PluginPermissionCatalogFilter) (PluginPermissionCatalog, error) {
	status := strings.TrimSpace(filter.Status)
	if status == "" {
		status = string(dbm.PermissionStatusActive)
	}
	var rows []dbm.Permission
	query := s.db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("source LIKE ?", "plugin:%").
		Where("status = ?", status).
		Order("source ASC, module ASC, resource ASC, action ASC")
	if pluginID := strings.TrimSpace(filter.PluginID); pluginID != "" {
		query = query.Where("source = ?", "plugin:"+pluginID)
	}
	if err := query.Find(&rows).Error; err != nil {
		return PluginPermissionCatalog{}, err
	}

	pluginMap := map[string]map[string]map[string][]PluginPermissionCatalogItem{}
	for _, row := range rows {
		var meta map[string]any
		_ = json.Unmarshal(row.Meta, &meta)
		itemType := strings.TrimSpace(utils.ToStr(meta["type"]))
		if itemType == "" {
			itemType = "action"
		}
		if requestedType := strings.TrimSpace(filter.Type); requestedType != "" && requestedType != itemType {
			continue
		}
		module := strings.TrimSpace(utils.ToStr(meta["module"]))
		if module == "" {
			module = row.Module
		}
		if requestedModule := strings.TrimSpace(filter.Module); requestedModule != "" && requestedModule != module {
			continue
		}
		pluginID := strings.TrimPrefix(strings.TrimSpace(row.Source), "plugin:")
		if pluginID == "" {
			pluginID = strings.TrimSpace(utils.ToStr(meta["plugin_id"]))
		}
		if pluginID == "" {
			continue
		}
		if _, ok := pluginMap[pluginID]; !ok {
			pluginMap[pluginID] = map[string]map[string][]PluginPermissionCatalogItem{}
		}
		if _, ok := pluginMap[pluginID][module]; !ok {
			pluginMap[pluginID][module] = map[string][]PluginPermissionCatalogItem{}
		}
		item := PluginPermissionCatalogItem{
			ID:                     row.ID,
			PermissionCode:         strings.TrimSpace(utils.ToStr(meta["permission"])),
			TitleI18n:              stringMapFromAny(meta["title_i18n"]),
			DescriptionI18n:        stringMapFromAny(meta["description_i18n"]),
			RiskLevel:              strings.TrimSpace(utils.ToStr(meta["risk_level"])),
			DataScope:              strings.TrimSpace(utils.ToStr(meta["data_scope"])),
			BusinessPermissionCode: strings.TrimSpace(utils.ToStr(meta["business_permission_code"])),
			ProtocolBindings:       meta["protocol_bindings"],
			DefaultRoleGrants:      stringSliceFromAny(meta["default_role_grants"]),
			Status:                 string(row.Status),
			RegistrationStatus:     pluginPermissionRegistrationStatus(meta),
		}
		if item.PermissionCode == "" {
			item.PermissionCode = row.Module + "." + row.Resource + ":" + row.Action
		}
		pluginMap[pluginID][module][itemType] = append(pluginMap[pluginID][module][itemType], item)
	}

	catalog := PluginPermissionCatalog{Plugins: make([]PluginPermissionCatalogPlugin, 0, len(pluginMap))}
	pluginIDs := make([]string, 0, len(pluginMap))
	for pluginID := range pluginMap {
		pluginIDs = append(pluginIDs, pluginID)
	}
	sort.Strings(pluginIDs)
	for _, pluginID := range pluginIDs {
		moduleNames := make([]string, 0, len(pluginMap[pluginID]))
		for module := range pluginMap[pluginID] {
			moduleNames = append(moduleNames, module)
		}
		sort.Strings(moduleNames)
		plugin := PluginPermissionCatalogPlugin{PluginID: pluginID, Modules: make([]PluginPermissionCatalogModule, 0, len(moduleNames))}
		for _, module := range moduleNames {
			typeNames := make([]string, 0, len(pluginMap[pluginID][module]))
			for typeName := range pluginMap[pluginID][module] {
				typeNames = append(typeNames, typeName)
			}
			sort.Strings(typeNames)
			moduleGroup := PluginPermissionCatalogModule{Module: module, Types: make([]PluginPermissionCatalogTypeGroup, 0, len(typeNames))}
			for _, typeName := range typeNames {
				moduleGroup.Types = append(moduleGroup.Types, PluginPermissionCatalogTypeGroup{
					Type:        typeName,
					Permissions: pluginMap[pluginID][module][typeName],
				})
			}
			plugin.Modules = append(plugin.Modules, moduleGroup)
		}
		catalog.Plugins = append(catalog.Plugins, plugin)
	}
	return catalog, nil
}

func pluginPermissionRegistrationStatus(meta map[string]any) string {
	if len(stringMapFromAny(meta["title_i18n"])) == 0 || len(stringMapFromAny(meta["description_i18n"])) == 0 {
		return "invalid"
	}
	if strings.TrimSpace(utils.ToStr(meta["type"])) == "" || strings.TrimSpace(utils.ToStr(meta["risk_level"])) == "" {
		return "invalid"
	}
	return "registered"
}

func stringMapFromAny(value any) map[string]string {
	rawMap, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(rawMap))
	for key, rawValue := range rawMap {
		key = strings.TrimSpace(key)
		value := strings.TrimSpace(utils.ToStr(rawValue))
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringSliceFromAny(value any) []string {
	rawSlice, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(rawSlice))
	seen := map[string]struct{}{}
	for _, rawValue := range rawSlice {
		value := strings.TrimSpace(utils.ToStr(rawValue))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
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
