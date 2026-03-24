package iam

import (
	"context"
	"encoding/json"
	"fmt"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"gorm.io/datatypes"
)

const (
	OpsPermissionModule = "platform_ops"

	OpsResourceDeploy    = "deploy"
	OpsResourcePlugin    = "plugin"
	OpsResourceBackup    = "backup"
	OpsResourceMigration = "migration"

	OpsActionRead     = "read"
	OpsActionExecute  = "execute"
	OpsActionRollback = "rollback"
	OpsActionApprove  = "approve"
)

type PermissionRegistrar interface {
	RegisterPermissions(ctx context.Context, rows []dbm.Permission) error
}

func EnsureOpsPermissions(ctx context.Context, registrar PermissionRegistrar) error {
	if registrar == nil {
		return fmt.Errorf("permission registrar is nil")
	}

	specs := []struct {
		Resource    string
		Action      string
		Label       string
		Description string
		Method      string
		Endpoint    string
	}{
		{OpsResourceDeploy, OpsActionRead, "Ops 部署-查看", "查看部署与健康状态", "GET", "/admin/deploy/*"},
		{OpsResourceDeploy, OpsActionExecute, "Ops 部署-发布", "触发部署发布动作", "POST", "/admin/deploy/releases"},
		{OpsResourceDeploy, OpsActionRollback, "Ops 部署-回滚", "触发部署回滚动作", "POST", "/admin/deploy/rollback"},
		{OpsResourcePlugin, OpsActionRead, "Ops 插件-查看", "查看插件生命周期审计", "GET", "/admin/plugins/*/audit"},
		{OpsResourcePlugin, OpsActionExecute, "Ops 插件-切换", "触发插件切换/回滚", "POST", "/admin/plugins/*/actions"},
		{OpsResourceBackup, OpsActionRead, "Ops 备份-查看", "查看备份策略与任务", "GET", "/admin/backup/*"},
		{OpsResourceBackup, OpsActionExecute, "Ops 备份-执行", "触发备份与恢复演练", "POST", "/admin/backup/*"},
		{OpsResourceMigration, OpsActionRead, "Ops 迁移-查看", "查看迁移流程与状态", "GET", "/admin/migration/*"},
		{OpsResourceMigration, OpsActionExecute, "Ops 迁移-执行", "触发迁移编排与切换", "POST", "/admin/migration/*"},
		{OpsResourceMigration, OpsActionApprove, "Ops 迁移-审批", "审批高风险迁移操作", "POST", "/admin/migration/*/approve"},
	}

	rows := make([]dbm.Permission, 0, len(specs))
	for _, spec := range specs {
		metaBytes, err := json.Marshal(map[string]any{
			"label":        spec.Label,
			"module":       "Platform Ops",
			"type":         "admin_action",
			"api_endpoint": spec.Endpoint,
			"http_method":  spec.Method,
		})
		if err != nil {
			return err
		}
		rows = append(rows, dbm.Permission{
			Module:      OpsPermissionModule,
			Resource:    spec.Resource,
			Action:      spec.Action,
			Description: spec.Description,
			Meta:        datatypes.JSON(metaBytes),
			Source:      OpsPermissionModule,
			Introduced:  "v0.1.0",
		})
	}

	return registrar.RegisterPermissions(ctx, rows)
}
