package registry

import (
	"context"
	"encoding/json"
	"fmt"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"gorm.io/datatypes"
)

const (
	PluginCapabilityRegistry = "capability_registry"

	ResourceCapability   = "capability"
	ResourceRouting      = "routing"
	ResourceFallbackPlan = "fallback_plan"

	ActionList      = "list"
	ActionView      = "view"
	ActionCreate    = "create"
	ActionUpdate    = "update"
	ActionDisable   = "disable"
	ActionSyncEvent = "sync_event"
	ActionManage    = "manage"
	ActionSandbox   = "sandbox"
)

const (
	ModuleCapabilityRegistry = "Capability Registry"
	TypeAdminAction          = "admin_action"

	ToolGrantAdmin     = "powerx.capability_registry.admin"
	ToolGrantSandbox   = "powerx.capability_registry.sandbox"
	SecurityScopeAdmin = "corex.capability_registry.admin"
)

// PermissionRegistrar 抽象出权限注册接口，便于注入 PermissionService。
type PermissionRegistrar interface {
	RegisterPermissions(ctx context.Context, rows []dbm.Permission) error
}

// EnsureAdminPermissions 将能力注册相关权限注册到 IAM 中，幂等执行。
func EnsureAdminPermissions(ctx context.Context, registrar PermissionRegistrar) error {
	if registrar == nil {
		return fmt.Errorf("permission registrar is nil")
	}

	perms, err := adminPermissions()
	if err != nil {
		return err
	}
	return registrar.RegisterPermissions(ctx, perms)
}

type permissionSpec struct {
	Resource    string
	Action      string
	Label       string
	Description string
	Method      string
	Endpoint    string
	ToolGrants  []string
}

func adminPermissions() ([]dbm.Permission, error) {
	specs := []permissionSpec{
		{
			Resource:    ResourceCapability,
			Action:      ActionList,
			Label:       "能力注册 - 列表查询",
			Description: "查看所有能力注册快照列表",
			Method:      "GET",
			Endpoint:    "/admin/capabilities",
			ToolGrants:  []string{ToolGrantAdmin},
		},
		{
			Resource:    ResourceCapability,
			Action:      ActionView,
			Label:       "能力注册 - 详情查看",
			Description: "查看单个能力注册详情",
			Method:      "GET",
			Endpoint:    "/admin/capabilities/{capabilityId}/tenants/{tenantId}",
			ToolGrants:  []string{ToolGrantAdmin},
		},
		{
			Resource:    ResourceCapability,
			Action:      ActionCreate,
			Label:       "能力注册 - 创建",
			Description: "创建或发布新的能力注册快照",
			Method:      "POST",
			Endpoint:    "/admin/capabilities",
			ToolGrants:  []string{ToolGrantAdmin},
		},
		{
			Resource:    ResourceCapability,
			Action:      ActionUpdate,
			Label:       "能力注册 - 更新",
			Description: "更新能力注册快照配置",
			Method:      "PUT",
			Endpoint:    "/admin/capabilities/{capabilityId}/tenants/{tenantId}",
			ToolGrants:  []string{ToolGrantAdmin},
		},
		{
			Resource:    ResourceCapability,
			Action:      ActionDisable,
			Label:       "能力注册 - 禁用",
			Description: "禁用能力注册以阻断调用",
			Method:      "DELETE",
			Endpoint:    "/admin/capabilities/{capabilityId}/tenants/{tenantId}",
			ToolGrants:  []string{ToolGrantAdmin},
		},
		{
			Resource:    ResourceRouting,
			Action:      ActionManage,
			Label:       "能力注册 - 路由策略管理",
			Description: "更新权重、限流与健康策略",
			Method:      "PUT",
			Endpoint:    "/admin/capabilities/{capabilityId}/tenants/{tenantId}/routing",
			ToolGrants:  []string{ToolGrantAdmin},
		},
		{
			Resource:    ResourceFallbackPlan,
			Action:      ActionManage,
			Label:       "能力注册 - Fallback 管理",
			Description: "配置能力的多级回退计划",
			Method:      "PUT",
			Endpoint:    "/admin/capabilities/{capabilityId}/tenants/{tenantId}/fallback",
			ToolGrants:  []string{ToolGrantAdmin},
		},
		{
			Resource:    ResourceRouting,
			Action:      ActionSyncEvent,
			Label:       "能力注册 - 推送事件",
			Description: "同步路由与健康状态事件",
			Method:      "POST",
			Endpoint:    "/admin/capabilities/{capabilityId}/tenants/{tenantId}/events",
			ToolGrants:  []string{ToolGrantAdmin},
		},
		{
			Resource:    ResourceRouting,
			Action:      ActionSandbox,
			Label:       "能力注册 - Sandbox 模拟",
			Description: "执行能力选路 Sandbox 演练",
			Method:      "POST",
			Endpoint:    "/admin/capabilities/{capabilityId}/sandbox",
			ToolGrants:  []string{ToolGrantAdmin, ToolGrantSandbox},
		},
	}

	perms := make([]dbm.Permission, 0, len(specs))
	for _, spec := range specs {
		metaBytes, err := json.Marshal(map[string]any{
			"label":        spec.Label,
			"module":       ModuleCapabilityRegistry,
			"type":         TypeAdminAction,
			"api_endpoint": spec.Endpoint,
			"http_method":  spec.Method,
			"tool_grants":  spec.ToolGrants,
		})
		if err != nil {
			return nil, err
		}

		perms = append(perms, dbm.Permission{
			Plugin:      PluginCapabilityRegistry,
			Resource:    spec.Resource,
			Action:      spec.Action,
			Description: spec.Description,
			Meta:        datatypes.JSON(metaBytes),
			Source:      PluginCapabilityRegistry,
			Introduced:  "v0.1.0",
		})
	}
	return perms, nil
}
