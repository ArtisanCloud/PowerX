// internal/service/iam/permission_service.go
package iam

import (
	"context"
	"encoding/json"
	"errors"
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
	PluginID        string                                  `json:"plugin_id"`
	MenuTree        []PluginPermissionCatalogMenuNode       `json:"menu_tree"`
	BusinessModules []PluginPermissionCatalogBusinessModule `json:"business_modules"`
	APIBindings     []PluginPermissionCatalogAPIBinding     `json:"api_bindings"`
}

type PluginPermissionCatalogMenuNode struct {
	Key                 string                            `json:"key"`
	LabelI18n           map[string]string                 `json:"label_i18n,omitempty"`
	Permission          *PluginPermissionCatalogItem      `json:"permission,omitempty"`
	PagePermissionCodes []string                          `json:"page_permission_codes,omitempty"`
	Children            []PluginPermissionCatalogMenuNode `json:"children,omitempty"`
}

type PluginPermissionCatalogBusinessModule struct {
	Module    string                                    `json:"module"`
	Resources []PluginPermissionCatalogBusinessResource `json:"resources"`
}

type PluginPermissionCatalogBusinessResource struct {
	Resource string                        `json:"resource"`
	Pages    []PluginPermissionCatalogItem `json:"pages,omitempty"`
	Actions  []PluginPermissionCatalogItem `json:"actions,omitempty"`
}

type PluginPermissionCatalogAPIBinding struct {
	BusinessPermissionCode string                      `json:"business_permission_code"`
	Independent            bool                        `json:"independent"`
	Permission             PluginPermissionCatalogItem `json:"permission"`
	ProtocolBindings       any                         `json:"protocol_bindings,omitempty"`
}

type PluginPermissionCatalogItem struct {
	ID                      uint64            `json:"id"`
	Type                    string            `json:"type"`
	PermissionCode          string            `json:"permission_code"`
	EffectivePermissionCode string            `json:"effective_permission_code"`
	Module                  string            `json:"module"`
	Resource                string            `json:"resource"`
	Action                  string            `json:"action"`
	MenuPath                []string          `json:"menu_path,omitempty"`
	PagePermissionCodes     []string          `json:"page_permission_codes,omitempty"`
	TitleI18n               map[string]string `json:"title_i18n"`
	DescriptionI18n         map[string]string `json:"description_i18n"`
	RiskLevel               string            `json:"risk_level"`
	DataScope               string            `json:"data_scope,omitempty"`
	BusinessPermissionCode  string            `json:"business_permission_code,omitempty"`
	ProtocolBindings        any               `json:"protocol_bindings,omitempty"`
	DefaultRoleGrants       []string          `json:"default_role_grants,omitempty"`
	Status                  string            `json:"status"`
	RegistrationStatus      string            `json:"registration_status"`
	RegistrationErrors      []string          `json:"registration_errors,omitempty"`
}

type PluginInvalidPermissionCleanupResult struct {
	PluginID             string   `json:"plugin_id"`
	DeletedPermissionIDs []uint64 `json:"deleted_permission_ids"`
	DeletedBindings      int64    `json:"deleted_bindings"`
	DeletedPermissions   int64    `json:"deleted_permissions"`
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

	pluginMap := map[string][]PluginPermissionCatalogItem{}
	for _, row := range rows {
		var meta map[string]any
		_ = json.Unmarshal(row.Meta, &meta)
		item := pluginPermissionCatalogItemFromRow(row, meta)
		if requestedType := strings.TrimSpace(filter.Type); requestedType != "" && requestedType != item.Type {
			continue
		}
		if requestedModule := strings.TrimSpace(filter.Module); requestedModule != "" && requestedModule != item.Module {
			continue
		}
		pluginID := strings.TrimPrefix(strings.TrimSpace(row.Source), "plugin:")
		if pluginID == "" {
			pluginID = strings.TrimSpace(utils.ToStr(meta["plugin_id"]))
		}
		if pluginID == "" {
			continue
		}
		pluginMap[pluginID] = append(pluginMap[pluginID], item)
	}

	catalog := PluginPermissionCatalog{Plugins: make([]PluginPermissionCatalogPlugin, 0, len(pluginMap))}
	pluginIDs := make([]string, 0, len(pluginMap))
	for pluginID := range pluginMap {
		pluginIDs = append(pluginIDs, pluginID)
	}
	sort.Strings(pluginIDs)
	for _, pluginID := range pluginIDs {
		items := pluginMap[pluginID]
		sortPluginPermissionCatalogItems(items)
		plugin := PluginPermissionCatalogPlugin{
			PluginID:        pluginID,
			MenuTree:        buildPluginPermissionMenuTree(items),
			BusinessModules: buildPluginPermissionBusinessModules(items),
			APIBindings:     buildPluginPermissionAPIBindings(items),
		}
		catalog.Plugins = append(catalog.Plugins, plugin)
	}
	return catalog, nil
}

func pluginPermissionCatalogItemFromRow(row dbm.Permission, meta map[string]any) PluginPermissionCatalogItem {
	itemType := strings.TrimSpace(utils.ToStr(meta["type"]))
	if itemType == "" {
		itemType = "action"
	}
	permissionCode := strings.TrimSpace(utils.ToStr(meta["permission"]))
	if permissionCode == "" {
		permissionCode = row.Module + "." + row.Resource + ":" + row.Action
	}
	module := strings.TrimSpace(utils.ToStr(meta["module"]))
	if module == "" {
		module = row.Module
	}
	resource := strings.TrimSpace(utils.ToStr(meta["resource"]))
	if resource == "" {
		resource = row.Resource
	}
	action := strings.TrimSpace(utils.ToStr(meta["action"]))
	if action == "" {
		action = row.Action
	}
	businessPermissionCode := strings.TrimSpace(utils.ToStr(meta["business_permission_code"]))
	independent := boolFromAny(meta["independent"])
	effectivePermissionCode := permissionCode
	if itemType == "api" && businessPermissionCode != "" {
		effectivePermissionCode = businessPermissionCode
	}
	item := PluginPermissionCatalogItem{
		ID:                      row.ID,
		Type:                    itemType,
		PermissionCode:          permissionCode,
		EffectivePermissionCode: effectivePermissionCode,
		Module:                  module,
		Resource:                resource,
		Action:                  action,
		MenuPath:                stringSliceFromAny(meta["menu_path"]),
		PagePermissionCodes:     stringSliceFromAny(meta["page_permission_codes"]),
		TitleI18n:               stringMapFromAny(meta["title_i18n"]),
		DescriptionI18n:         stringMapFromAny(meta["description_i18n"]),
		RiskLevel:               strings.TrimSpace(utils.ToStr(meta["risk_level"])),
		DataScope:               strings.TrimSpace(utils.ToStr(meta["data_scope"])),
		BusinessPermissionCode:  businessPermissionCode,
		ProtocolBindings:        meta["protocol_bindings"],
		DefaultRoleGrants:       stringSliceFromAny(meta["default_role_grants"]),
		Status:                  string(row.Status),
	}
	item.RegistrationErrors = dedupeSortedIAMStrings(append(
		pluginPermissionRegistrationErrors(item, independent),
		stringSliceFromAny(meta["registration_errors"])...,
	))
	if len(item.RegistrationErrors) > 0 {
		item.RegistrationStatus = "invalid"
	} else {
		item.RegistrationStatus = "registered"
	}
	return item
}

func pluginPermissionRegistrationStatus(meta map[string]any) string {
	item := pluginPermissionCatalogItemFromRow(dbm.Permission{}, meta)
	return item.RegistrationStatus
}

func pluginPermissionRegistrationErrors(item PluginPermissionCatalogItem, independent bool) []string {
	errors := make([]string, 0)
	if item.PermissionCode == "" {
		errors = append(errors, "permission_code_missing")
	}
	if len(item.TitleI18n) == 0 {
		errors = append(errors, "title_i18n_missing")
	}
	if len(item.DescriptionI18n) == 0 {
		errors = append(errors, "description_i18n_missing")
	}
	if item.Type == "" {
		errors = append(errors, "type_missing")
	}
	if item.Module == "" {
		errors = append(errors, "module_missing")
	}
	if item.RiskLevel == "" {
		errors = append(errors, "risk_level_missing")
	}
	if item.DataScope == "" {
		errors = append(errors, "data_scope_missing")
	}
	if pluginPermissionContainsPluginID(item.PermissionCode) ||
		pluginPermissionContainsPluginID(item.Module) ||
		pluginPermissionContainsPluginID(item.Resource) ||
		pluginPermissionContainsPluginID(item.Action) ||
		pluginPermissionPathContainsPluginID(item.MenuPath) {
		errors = append(errors, "plugin_id_in_business_permission")
	}
	if item.Module == "menu" {
		errors = append(errors, "module_menu_invalid")
	}
	if item.Type == "menu" {
		if len(item.MenuPath) == 0 {
			errors = append(errors, "menu_path_missing")
		}
		if pluginPermissionMenuPathHasTechnicalSegment(item.MenuPath) {
			errors = append(errors, "menu_path_invalid_technical_segment")
		}
		if !strings.HasPrefix(item.PermissionCode, "menu.") {
			errors = append(errors, "menu_permission_code_invalid")
		}
	}
	if item.Type == "page" || item.Type == "action" || item.Type == "api" {
		if item.Resource == "" {
			errors = append(errors, "resource_missing")
		}
		if item.Action == "" {
			errors = append(errors, "action_missing")
		}
		if item.Module != "" && item.Resource != "" && item.Action != "" {
			expected := item.Module + "." + item.Resource + ":" + item.Action
			if item.PermissionCode != expected {
				errors = append(errors, "permission_code_mismatch")
			}
		}
	}
	if item.Type == "api" {
		if item.ProtocolBindings == nil {
			errors = append(errors, "protocol_bindings_missing")
		}
		if item.BusinessPermissionCode == "" && !independent {
			errors = append(errors, "business_permission_code_missing")
		}
	}
	if item.Type != "menu" && item.Type != "page" && item.Type != "action" && item.Type != "api" {
		errors = append(errors, "type_invalid")
	}
	return dedupeSortedIAMStrings(errors)
}

func pluginPermissionContainsPluginID(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(value, "com.powerx.plugin") ||
		strings.Contains(value, "com.powerx.plugins") ||
		strings.Contains(value, "plugin.")
}

func pluginPermissionPathContainsPluginID(values []string) bool {
	for _, value := range values {
		if pluginPermissionContainsPluginID(value) {
			return true
		}
	}
	return false
}

func pluginPermissionMenuPathHasTechnicalSegment(values []string) bool {
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if strings.Contains(value, "/_p/") ||
			strings.HasPrefix(value, "_p/") ||
			strings.Contains(value, "/api/v1") ||
			strings.HasPrefix(value, "api/v1") ||
			strings.HasPrefix(value, "http://") ||
			strings.HasPrefix(value, "https://") {
			return true
		}
	}
	return false
}

func sortPluginPermissionCatalogItems(items []PluginPermissionCatalogItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Type != b.Type {
			return pluginPermissionTypeOrder(a.Type) < pluginPermissionTypeOrder(b.Type)
		}
		if a.Module != b.Module {
			return a.Module < b.Module
		}
		if a.Resource != b.Resource {
			return a.Resource < b.Resource
		}
		if a.Action != b.Action {
			return a.Action < b.Action
		}
		return a.PermissionCode < b.PermissionCode
	})
}

func pluginPermissionTypeOrder(value string) int {
	switch value {
	case "menu":
		return 0
	case "page":
		return 1
	case "action":
		return 2
	case "api":
		return 3
	default:
		return 9
	}
}

func buildPluginPermissionMenuTree(items []PluginPermissionCatalogItem) []PluginPermissionCatalogMenuNode {
	roots := make([]PluginPermissionCatalogMenuNode, 0)
	for _, item := range items {
		if item.Type != "menu" {
			continue
		}
		path := item.MenuPath
		if len(path) == 0 {
			path = []string{item.PermissionCode}
		}
		insertPluginPermissionMenuNode(&roots, path, item, 0)
	}
	sortPluginPermissionMenuNodes(roots)
	return roots
}

func insertPluginPermissionMenuNode(nodes *[]PluginPermissionCatalogMenuNode, path []string, item PluginPermissionCatalogItem, depth int) {
	if depth >= len(path) {
		return
	}
	key := strings.TrimSpace(path[depth])
	if key == "" {
		return
	}
	for idx := range *nodes {
		if (*nodes)[idx].Key == key {
			if depth == len(path)-1 {
				copied := item
				(*nodes)[idx].Permission = &copied
				(*nodes)[idx].LabelI18n = item.TitleI18n
				(*nodes)[idx].PagePermissionCodes = item.PagePermissionCodes
			}
			insertPluginPermissionMenuNode(&(*nodes)[idx].Children, path, item, depth+1)
			return
		}
	}
	node := PluginPermissionCatalogMenuNode{Key: key}
	if depth == len(path)-1 {
		copied := item
		node.Permission = &copied
		node.LabelI18n = item.TitleI18n
		node.PagePermissionCodes = item.PagePermissionCodes
	}
	*nodes = append(*nodes, node)
	insertPluginPermissionMenuNode(&(*nodes)[len(*nodes)-1].Children, path, item, depth+1)
}

func sortPluginPermissionMenuNodes(nodes []PluginPermissionCatalogMenuNode) {
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].Key < nodes[j].Key })
	for idx := range nodes {
		sortPluginPermissionMenuNodes(nodes[idx].Children)
	}
}

func buildPluginPermissionBusinessModules(items []PluginPermissionCatalogItem) []PluginPermissionCatalogBusinessModule {
	moduleMap := map[string]map[string]*PluginPermissionCatalogBusinessResource{}
	for _, item := range items {
		if item.Type != "page" && item.Type != "action" {
			continue
		}
		if _, ok := moduleMap[item.Module]; !ok {
			moduleMap[item.Module] = map[string]*PluginPermissionCatalogBusinessResource{}
		}
		resource := moduleMap[item.Module][item.Resource]
		if resource == nil {
			resource = &PluginPermissionCatalogBusinessResource{Resource: item.Resource}
			moduleMap[item.Module][item.Resource] = resource
		}
		if item.Type == "page" {
			resource.Pages = append(resource.Pages, item)
		} else {
			resource.Actions = append(resource.Actions, item)
		}
	}
	moduleNames := make([]string, 0, len(moduleMap))
	for module := range moduleMap {
		moduleNames = append(moduleNames, module)
	}
	sort.Strings(moduleNames)
	out := make([]PluginPermissionCatalogBusinessModule, 0, len(moduleNames))
	for _, module := range moduleNames {
		resourceNames := make([]string, 0, len(moduleMap[module]))
		for resource := range moduleMap[module] {
			resourceNames = append(resourceNames, resource)
		}
		sort.Strings(resourceNames)
		group := PluginPermissionCatalogBusinessModule{Module: module, Resources: make([]PluginPermissionCatalogBusinessResource, 0, len(resourceNames))}
		for _, resourceName := range resourceNames {
			resource := moduleMap[module][resourceName]
			sortPluginPermissionCatalogItems(resource.Pages)
			sortPluginPermissionCatalogItems(resource.Actions)
			group.Resources = append(group.Resources, *resource)
		}
		out = append(out, group)
	}
	return out
}

func buildPluginPermissionAPIBindings(items []PluginPermissionCatalogItem) []PluginPermissionCatalogAPIBinding {
	out := make([]PluginPermissionCatalogAPIBinding, 0)
	for _, item := range items {
		if item.Type != "api" {
			continue
		}
		out = append(out, PluginPermissionCatalogAPIBinding{
			BusinessPermissionCode: item.BusinessPermissionCode,
			Independent:            item.BusinessPermissionCode == "",
			Permission:             item,
			ProtocolBindings:       item.ProtocolBindings,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].BusinessPermissionCode != out[j].BusinessPermissionCode {
			return out[i].BusinessPermissionCode < out[j].BusinessPermissionCode
		}
		return out[i].Permission.PermissionCode < out[j].Permission.PermissionCode
	})
	return out
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func dedupeSortedIAMStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func (s *PermissionService) CleanupInvalidPluginPermissions(ctx context.Context, pluginID string) (PluginInvalidPermissionCleanupResult, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return PluginInvalidPermissionCleanupResult{}, errors.New("iam.permission.plugin_id_required")
	}
	source := "plugin:" + pluginID
	var rows []dbm.Permission
	if err := s.db.WithContext(ctx).
		Where("source = ? AND status = ?", source, dbm.PermissionStatusActive).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return PluginInvalidPermissionCleanupResult{}, err
	}

	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		var meta map[string]any
		_ = json.Unmarshal(row.Meta, &meta)
		if pluginPermissionRegistrationStatus(meta) != "registered" {
			ids = append(ids, row.ID)
		}
	}
	result := PluginInvalidPermissionCleanupResult{
		PluginID:             pluginID,
		DeletedPermissionIDs: ids,
	}
	if len(ids) == 0 {
		return result, nil
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bindings := tx.Where("permission_id IN ?", ids).Delete(&dbm.RolePermission{})
		if bindings.Error != nil {
			return bindings.Error
		}
		perms := tx.Where("id IN ?", ids).Delete(&dbm.Permission{})
		if perms.Error != nil {
			return perms.Error
		}
		result.DeletedBindings = bindings.RowsAffected
		result.DeletedPermissions = perms.RowsAffected
		return nil
	})
	if err != nil {
		return PluginInvalidPermissionCleanupResult{}, err
	}
	return result, nil
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
	if rawStrings, ok := value.([]string); ok {
		return dedupeSortedIAMStrings(rawStrings)
	}
	rawSlice, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(rawSlice))
	for _, rawValue := range rawSlice {
		value := strings.TrimSpace(utils.ToStr(rawValue))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return dedupeSortedIAMStrings(out)
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
