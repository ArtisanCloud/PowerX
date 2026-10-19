package authorizationbaseline

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/rootrole"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const AutomaticInitializer = "core.authorization-baseline"

// EnsureInitialized 在迁移完成或正常启动时补齐所有适用租户的授权追踪。
// 维护系统 root 目录并处理声明了 authorization_baseline 的权限；不修改业务角色、插件数据或既有起点。
// 首次安装尚未建表时无操作；已建部分表的损坏现场明确失败。
func EnsureInitialized(db *gorm.DB) error {
	if db == nil {
		return errors.New("nil database")
	}
	if !SupportedDatabase(db) || !db.Migrator().HasTable(&im.AuthorizationTracking{}) {
		return nil
	}
	if !db.Migrator().HasTable(&im.AuthorizationStream{}) || !db.Migrator().HasTable(&im.AuthorizationSegment{}) {
		return errors.New("IAM_BASELINE_SCHEMA_INCOMPLETE")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var coordinated *trackedTransaction
		wasDirty := false
		if Managed(tx) {
			coordinated = tx.Statement.ConnPool.(*trackedTransaction)
			wasDirty = coordinated.dirty
			if err := Lock(tx); err != nil {
				return err
			}
		} else if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(704983720161015)").Error; err != nil {
				return err
			}
		}
		_, err := rootrole.Ensure(tx)
		if err != nil {
			return err
		}
		at, err := transactionTime(tx)
		if err != nil {
			return err
		}
		tracks, err := initializeMissing(tx, at)
		if err != nil {
			return err
		}
		// 启动/迁移只补新登记；既有流发生外部漂移时仍由读取明确报错，不能伪造增量历史。
		if err := reconcileTracks(tx, at, tracks); err != nil {
			return err
		}
		if err := upgradeRootSources(tx, at); err != nil {
			return err
		}
		if coordinated != nil && !wasDirty {
			// 本次仅有受控 root 目录及账本维护，已按精确旧指纹迁移。
			// 不让启动时的目录补齐顺带修复其他未知漂移；外层原有源变更仍正常 Commit。
			coordinated.dirty = false
		}
		return nil
	})
}

// upgradeRootSources 只迁移能与旧算法精确对应的 root 流，从本次升级时间
// 追加正式来源。其他漂移不被猜测修复；既有 known_since/历史段保持不变。
func upgradeRootSources(db *gorm.DB, at time.Time) error {
	var tracks []im.AuthorizationTracking
	if err := db.Where("status = ?", "ready").Find(&tracks).Error; err != nil {
		return err
	}
	return reconcileTracksWhere(db, at, tracks, func(sources []Source, root bool, stream im.AuthorizationStream, exists bool) bool {
		if !exists || !root {
			return false
		}
		legacy := make([]Source, 0, len(sources))
		hasRoot := false
		for _, source := range sources {
			if strings.HasPrefix(source.ID, rootSourcePrefix) {
				hasRoot = true
			} else {
				legacy = append(legacy, source)
			}
		}
		return hasRoot && stream.FactsHash == Digest([]any{legacy, root})
	})
}

func transactionTime(db *gorm.DB) (time.Time, error) {
	at := Micro(time.Now())
	if db.Dialector.Name() == "postgres" {
		if err := db.Raw("SELECT clock_timestamp()").Scan(&at).Error; err != nil {
			return time.Time{}, err
		}
	}
	return Micro(at), nil
}

// initializeMissing 与授权源写入使用同一事务和同一平台锁。
// 每个新记录的初始快照在同一 at 生成；已有记录（包括不可用状态）不被覆盖或猜测修复。
func initializeMissing(db *gorm.DB, at time.Time) ([]im.AuthorizationTracking, error) {
	var permissions []im.Permission
	// 固定所需列，避免 schema 迁移增列时 PostgreSQL SELECT * 缓存计划失效。
	if err := db.Select("module", "resource", "action", "source", "meta").
		Where("source LIKE ? AND status = ?", "plugin:%", im.PermissionStatusActive).Find(&permissions).Error; err != nil {
		return nil, err
	}
	byPlugin := map[string]map[string]bool{}
	for _, permission := range permissions {
		var meta map[string]any
		if json.Unmarshal(permission.Meta, &meta) != nil || meta["authorization_baseline"] != true {
			continue
		}
		plugin := strings.TrimSpace(strings.TrimPrefix(permission.Source, "plugin:"))
		code := pluginpermission.EffectivePluginPermissionCode(permission)
		if plugin == "" || code == "" {
			continue
		}
		if byPlugin[plugin] == nil {
			byPlugin[plugin] = map[string]bool{}
		}
		byPlugin[plugin][code] = true
	}
	if len(byPlugin) == 0 {
		return nil, nil
	}
	var existing []im.AuthorizationTracking
	if err := db.Select("tenant_uuid", "plugin_id", "permission_code").Find(&existing).Error; err != nil {
		return nil, err
	}
	type key struct{ tenant, plugin, permission string }
	registered := map[key]bool{}
	for _, row := range existing {
		registered[key{row.TenantUUID, row.PluginID, row.PermissionCode}] = true
	}
	var tenants []tm.Tenant
	if err := db.Select("uuid").Where("status = ?", tm.TenantStatusActive).Find(&tenants).Error; err != nil {
		return nil, err
	}
	active := map[string]bool{}
	for _, tenant := range tenants {
		active[tenant.UUID.String()] = true
	}
	var instances []sm.PluginInstanceConfig
	if err := db.Select("tenant_uuid", "plugin_id").Where("key = ? AND enabled = ?", "auth.credentials", true).
		Order("tenant_uuid, plugin_id").Find(&instances).Error; err != nil {
		return nil, err
	}
	var created []im.AuthorizationTracking
	for _, instance := range instances {
		if !active[instance.TenantUUID] {
			continue
		}
		codes := make([]string, 0, len(byPlugin[instance.PluginID]))
		for code := range byPlugin[instance.PluginID] {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		for _, code := range codes {
			if registered[key{instance.TenantUUID, instance.PluginID, code}] {
				continue
			}
			row := im.AuthorizationTracking{TenantUUID: instance.TenantUUID, PluginID: instance.PluginID, PermissionCode: code,
				Status: "ready", KnownSince: &at, InitializationGuardUUID: uuid.NewString(), InitializedBy: AutomaticInitializer}
			result := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_uuid"}, {Name: "plugin_id"}, {Name: "permission_code"}}, DoNothing: true}).Create(&row)
			if result.Error != nil {
				return nil, result.Error
			}
			if result.RowsAffected > 0 {
				created = append(created, row)
			}
			registered[key{instance.TenantUUID, instance.PluginID, code}] = true
		}
	}
	return created, nil
}
