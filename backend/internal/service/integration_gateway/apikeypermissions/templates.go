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
	return repo.UpsertBatch(ctx, BuildTemplatePermissions())
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
		profPermRepo := iamrepo.NewAPIKeyProfilePermissionRepository(tx)

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

		currentIDs, listErr := profPermRepo.ListPermissionIDsOfProfile(ctx, existed.ID)
		if listErr != nil {
			return listErr
		}
		toAdd, toRemove := diffPermissionIDs(currentIDs, permissionIDs)
		if revokeErr := profPermRepo.RevokeByIDsTx(tx, existed.ID, toRemove); revokeErr != nil {
			return revokeErr
		}
		if grantErr := profPermRepo.GrantByIDsTx(tx, existed.ID, toAdd); grantErr != nil {
			return grantErr
		}
		profile = existed
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
	return profile, permissionIDs, nil
}

func diffPermissionIDs(current []uint64, desired []uint64) (toAdd []uint64, toRemove []uint64) {
	currentSet := make(map[uint64]struct{}, len(current))
	desiredSet := make(map[uint64]struct{}, len(desired))
	for _, id := range current {
		currentSet[id] = struct{}{}
	}
	for _, id := range desired {
		desiredSet[id] = struct{}{}
	}
	for _, id := range desired {
		if _, ok := currentSet[id]; !ok {
			toAdd = append(toAdd, id)
		}
	}
	for _, id := range current {
		if _, ok := desiredSet[id]; !ok {
			toRemove = append(toRemove, id)
		}
	}
	return
}

func BuildTemplatePermissions() []modelsiam.Permission {
	return []modelsiam.Permission{
		build("integration_gateway", "api_key.ws.system_notification", "subscribe", "API Key：WS 订阅系统通知", map[string]string{
			"scope": "_scope.ws.topic.subscribe", "action": "subscribe", "resource_type": "topic", "resource_pattern": "_topic.system.notification",
		}),
		build("integration_gateway", "api_key.ws.system_notification", "publish", "API Key：WS 发布系统通知", map[string]string{
			"scope": "_scope.ws.topic.publish", "action": "publish", "resource_type": "topic", "resource_pattern": "_topic.system.notification",
		}),
		build("integration_gateway", "api_key.event.system_notification", "publish", "API Key：Event 发布系统通知", map[string]string{
			"scope": "_scope.event.topic.publish", "action": "publish", "resource_type": "topic", "resource_pattern": "_topic.system.notification",
		}),
		build("integration_gateway", "api_key.event.system_notification", "subscribe", "API Key：Event 订阅系统通知", map[string]string{
			"scope": "_scope.event.topic.subscribe", "action": "subscribe", "resource_type": "topic", "resource_pattern": "_topic.system.notification",
		}),
		build("integration_gateway", "api_key.event.knowledge_feedback_reprocess", "publish", "API Key：Event 发布知识回放主题", map[string]string{
			"scope": "_scope.event.topic.publish", "action": "publish", "resource_type": "topic", "resource_pattern": "_topic.knowledge.space.feedback.reprocess",
		}),
		build("integration_gateway", "api_key.event.knowledge_feedback_reprocess", "replay", "API Key：Event 回放知识回放主题", map[string]string{
			"scope": "_scope.event.topic.replay", "action": "replay", "resource_type": "topic", "resource_pattern": "_topic.knowledge.space.feedback.reprocess",
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
