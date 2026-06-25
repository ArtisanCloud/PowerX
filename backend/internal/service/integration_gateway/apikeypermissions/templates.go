package apikeypermissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	modelsiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"gorm.io/gorm"
)

func EnsureTemplatePermissions(ctx context.Context, repo *iamrepo.PermissionRepository) error {
	if repo == nil {
		return nil
	}
	rows := BuildTemplatePermissions()
	if platformRows, err := BuildPlatformCapabilityPermissions(); err == nil {
		rows = append(rows, platformRows...)
	}
	return repo.UpsertBatch(ctx, rows)
}

const (
	DefaultAPIKeyProfileKey  = "integration.default"
	DefaultAPIKeyProfileName = "Integration Default API Key Profile"
)

func EnsureTenantDefaultProfile(ctx context.Context, db *gorm.DB, tenantUUID string, ownerMemberID *uint64) (*modelsiam.APIKeyProfile, []uint64, error) {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return nil, nil, fmt.Errorf("tenant_uuid required")
	}
	if db == nil {
		return nil, nil, fmt.Errorf("db required")
	}
	permRepo := iamrepo.NewPermissionRepository(db)
	if err := EnsureTemplatePermissions(ctx, permRepo); err != nil {
		return nil, nil, err
	}
	rows, _, err := permRepo.List(ctx, map[string]string{
		"status":        string(modelsiam.PermissionStatusActive),
		"allow_api_key": "true",
		"module":        "integration_gateway",
	}, 0, 5000, "id ASC")
	if err != nil {
		return nil, nil, err
	}
	permissionIDs := make([]uint64, 0, len(rows))
	for i := range rows {
		permissionIDs = append(permissionIDs, rows[i].ID)
	}
	profiles := iamrepo.NewAPIKeyProfileRepository(db)
	var profile *modelsiam.APIKeyProfile
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		profileRepo := iamrepo.NewAPIKeyProfileRepository(tx)
		profilePermRepo := iamrepo.NewAPIKeyProfilePermissionRepository(tx)

		existed, findErr := profileRepo.FindByKey(ctx, tenantUUID, DefaultAPIKeyProfileKey)
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if existed == nil {
			created, createErr := profileRepo.Create(ctx, &modelsiam.APIKeyProfile{
				TenantUUID:    tenantUUID,
				OwnerMemberID: ownerMemberID,
				Key:           DefaultAPIKeyProfileKey,
				Name:          DefaultAPIKeyProfileName,
				Status:        1,
			})
			if createErr != nil {
				return createErr
			}
			existed = created
		} else {
			changed := false
			if existed.Status != 1 {
				existed.Status = 1
				changed = true
			}
			if strings.TrimSpace(existed.Name) == "" {
				existed.Name = DefaultAPIKeyProfileName
				changed = true
			}
			if existed.OwnerMemberID == nil && ownerMemberID != nil && *ownerMemberID > 0 {
				value := *ownerMemberID
				existed.OwnerMemberID = &value
				changed = true
			}
			if changed {
				updated, updateErr := profileRepo.Update(ctx, existed)
				if updateErr != nil {
					return updateErr
				}
				existed = updated
			}
		}

		profile = existed
		if profile != nil && profile.ID > 0 {
			if grantErr := profilePermRepo.GrantByIDsTx(tx, profile.ID, permissionIDs); grantErr != nil {
				return grantErr
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if profile == nil {
		profile, err = profiles.FindByKey(ctx, tenantUUID, DefaultAPIKeyProfileKey)
		if err != nil {
			return nil, nil, err
		}
	}
	currentIDs := []uint64{}
	if profile != nil && profile.ID > 0 {
		currentIDs, err = iamrepo.NewAPIKeyProfilePermissionRepository(db).ListPermissionIDsOfProfile(ctx, profile.ID)
		if err != nil {
			return nil, nil, err
		}
	}
	if len(currentIDs) == 0 {
		currentIDs = permissionIDs
	}
	return profile, currentIDs, nil
}

func BuildTemplatePermissions() []modelsiam.Permission {
	return []modelsiam.Permission{
		build("integration_gateway", "api_key.ws.topic", "subscribe", "API Key：WS 订阅 Topic（通用）", map[string]string{
			"scope": "_scope.ws.topic.subscribe", "action": "subscribe", "resource_type": "topic", "resource_pattern": "*",
		}),
		build("integration_gateway", "api_key.ws.topic", "publish", "API Key：WS 发布 Topic（通用）", map[string]string{
			"scope": "_scope.ws.topic.publish", "action": "publish", "resource_type": "topic", "resource_pattern": "*",
		}),
		build("integration_gateway", "api_key.event.topic", "publish", "API Key：Event 发布 Topic（通用）", map[string]string{
			"scope": "_scope.event.topic.publish", "action": "publish", "resource_type": "topic", "resource_pattern": "*",
		}),
		build("integration_gateway", "api_key.event.topic", "subscribe", "API Key：Event 订阅 Topic（通用）", map[string]string{
			"scope": "_scope.event.topic.subscribe", "action": "subscribe", "resource_type": "topic", "resource_pattern": "*",
		}),
		build("integration_gateway", "api_key.event.topic", "replay", "API Key：Event 回放 Topic（通用）", map[string]string{
			"scope": "_scope.event.topic.replay", "action": "replay", "resource_type": "topic", "resource_pattern": "*",
		}),
		build("integration_gateway", "api_key.iam.organization_departments", "list", "API Key：组织架构-部门树只读", map[string]string{
			"scope": "_scope.iam.organization.department.list", "action": "list", "resource_type": "api", "resource_pattern": "GET:/api/v1/admin/organization/departments/tree",
		}),
		build("integration_gateway", "api_key.iam.member", "list", "API Key：组织架构-成员列表只读", map[string]string{
			"scope": "_scope.iam.member.list", "action": "list", "resource_type": "api", "resource_pattern": "GET:/api/v1/admin/iam/members",
		}),
		build("integration_gateway", "api_key.iam.member", "read", "API Key：组织架构-成员详情只读", map[string]string{
			"scope": "_scope.iam.member.read", "action": "read", "resource_type": "api", "resource_pattern": "GET:/api/v1/admin/iam/members/:id",
		}),
		build("integration_gateway", "api_key.agent", "invoke", "API Key：Agent 对话调用", map[string]string{
			"scope": "_scope.agent.invoke", "action": "invoke", "resource_type": "api", "resource_pattern": "POST:/api/v1/agents/invoke",
		}),
		build("integration_gateway", "api_key.agent", "stream", "API Key：Agent SSE 流式调用", map[string]string{
			"scope": "_scope.agent.stream", "action": "stream", "resource_type": "api", "resource_pattern": "GET:/api/v1/agents/stream/sse",
		}),
		build("integration_gateway", "api_key.agent.session", "manage", "API Key：Agent 会话管理", map[string]string{
			"scope": "_scope.agent.session.manage", "action": "manage", "resource_type": "api", "resource_pattern": "POST:/api/v1/agents/sessions",
		}),
		build("integration_gateway", "api_key.plugin.skill_registry", "sync", "API Key：插件 Skill 注册同步", map[string]string{
			"scope": "_scope.plugin.skill_registry.sync", "action": "sync", "resource_type": "api", "resource_pattern": "POST:/api/v1/admin/skills/plugin-registry*",
		}),
		build("integration_gateway", "api_key.plugin.capability_catalog", "sync", "API Key：插件 Capability Catalog 同步", map[string]string{
			"scope": "_scope.plugin.capability_catalog.sync", "action": "sync", "resource_type": "api", "resource_pattern": "POST:/api/v1/internal/plugins/capabilities/catalog",
		}),
		build("integration_gateway", "api_key.plugin.agent_registry", "sync", "API Key：插件 Agent 注册同步", map[string]string{
			"scope": "_scope.plugin.agent_registry.sync", "action": "sync", "resource_type": "api", "resource_pattern": "POST:/api/v1/admin/agents*",
		}),
		build("integration_gateway", "api_key.plugin.agent_registry", "update", "API Key：插件 Agent 注册更新同步", map[string]string{
			"scope": "_scope.plugin.agent_registry.sync", "action": "sync", "resource_type": "api", "resource_pattern": "PATCH:/api/v1/admin/agents*",
		}),
	}
}

func build(module string, resource string, action string, description string, apiKeyMeta map[string]string) modelsiam.Permission {
	meta := map[string]any{
		"type":   "api_key",
		"module": "integration_gateway",
		"label":  description,
		"api_key": map[string]any{
			"scope":            strings.TrimSpace(apiKeyMeta["scope"]),
			"action":           strings.TrimSpace(apiKeyMeta["action"]),
			"resource_type":    strings.TrimSpace(apiKeyMeta["resource_type"]),
			"resource_pattern": strings.TrimSpace(apiKeyMeta["resource_pattern"]),
			"effect":           "allow",
		},
	}
	metaBytes, _ := json.Marshal(meta)
	return modelsiam.Permission{
		Module:      module,
		Resource:    resource,
		Action:      action,
		Effect:      "allow",
		Description: description,
		AllowAPIKey: true,
		Meta:        metaBytes,
		Status:      modelsiam.PermissionStatusActive,
		Source:      "integration_gateway",
		Introduced:  IntroducedVersion(),
	}
}
