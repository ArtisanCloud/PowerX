package authorizationbaseline

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	ar "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/authorization"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/rootrole"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const RootCatalogRepairContract = "core.iam.root_catalog_repair.v2"

var ErrRootRepairPlanConflict = errors.New("IAM_ROOT_CATALOG_REPAIR_PLAN_CONFLICT")
var ErrRootRepairBlocked = errors.New("IAM_ROOT_CATALOG_REPAIR_BLOCKED")

type RootCatalogRepairInput struct {
	Apply               bool
	ExpectedFingerprint string
	Operator            string
}

type RootCatalogRepairLink struct {
	PermissionID   uint64    `json:"permission_id"`
	PermissionUUID uuid.UUID `json:"permission_uuid"`
	OldRoleUUID    uuid.UUID `json:"old_role_uuid"`
	CreatedAt      int64     `json:"created_at"`
	Reason         string    `json:"reason"`
}

type RootCatalogRepairPlan struct {
	Contract       string                     `json:"contract"`
	RootRoleID     uint64                     `json:"root_role_id"`
	RootUUID       uuid.UUID                  `json:"root_uuid"`
	Fingerprint    string                     `json:"fingerprint"`
	Candidates     []RootCatalogRepairLink    `json:"candidates"`
	Blocked        []RootCatalogRepairLink    `json:"blocked"`
	History        []RootCatalogRepairHistory `json:"history"`
	HistoryBlocked []RootCatalogRepairHistory `json:"history_blocked"`
	TotalLinks     int                        `json:"total_links"`
	Applied        bool                       `json:"applied"`
	Repaired       int                        `json:"repaired"`
	Operator       string                     `json:"operator,omitempty"`
	AppliedAt      *time.Time                 `json:"applied_at,omitempty"`
}

type rootCatalogReference struct {
	im.RolePermission
	CanonicalPermissionUUID uuid.UUID
	PermissionMeta          string
	PermissionEffect        string
	PermissionStatus        string
	PermissionDeleted       bool
	RoleUUIDExists          bool
}

// RepairRootCatalog 仅供显式维护命令调用。预览使用只读快照；执行核对完整
// 目录指纹，只重绑无任何旧角色归属且权限 UUID 一致的系统 Root 目录行。
// 不进入启动、迁移、GET 或普通权限同步，不解释或改写业务角色授权。
func RepairRootCatalog(ctx context.Context, db *gorm.DB, in RootCatalogRepairInput) (*RootCatalogRepairPlan, error) {
	if db == nil || !SupportedDatabase(db) {
		return nil, ErrUnsupportedDatabase
	}
	if in.Apply && (strings.TrimSpace(in.Operator) == "" || len(in.ExpectedFingerprint) != 64) {
		return nil, errors.New("IAM_ROOT_CATALOG_REPAIR_CONFIRMATION_REQUIRED")
	}
	var plan *RootCatalogRepairPlan
	inspect := func(tx *gorm.DB) error {
		var err error
		plan, err = inspectRootCatalog(tx, in.Apply)
		return err
	}
	if !in.Apply {
		err := db.WithContext(ctx).Transaction(inspect, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
		return plan, err
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		coordinated, ok := tx.Statement.ConnPool.(*trackedTransaction)
		if !ok || coordinated.dirty {
			return ErrWriteTransaction
		}
		if err := Lock(tx); err != nil {
			return err
		}
		if err := inspect(tx); err != nil {
			return err
		}
		if plan.Fingerprint != in.ExpectedFingerprint {
			return ErrRootRepairPlanConflict
		}
		if len(plan.Blocked) > 0 {
			return ErrRootRepairBlocked
		}
		if len(plan.HistoryBlocked) > 0 {
			return errors.New("IAM_ROOT_CATALOG_REPAIR_HISTORY_CONFLICT")
		}
		for _, item := range plan.Candidates {
			result := tx.Model(&im.RolePermission{}).
				Where("role_id=? AND permission_id=? AND role_uuid=? AND permission_uuid=? AND created_at=?", plan.RootRoleID, item.PermissionID, item.OldRoleUUID, item.PermissionUUID, item.CreatedAt).
				Where("NOT EXISTS (SELECT 1 FROM "+(&im.Role{}).TableName()+" old_role WHERE old_role.uuid=?)", item.OldRoleUUID).
				Update("role_uuid", plan.RootUUID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrRootRepairPlanConflict
			}
		}
		after, err := inspectRootCatalog(tx, true)
		if err != nil {
			return err
		}
		if len(after.Candidates)+len(after.Blocked) != 0 || after.TotalLinks != plan.TotalLinks {
			return ErrRootRepairPlanConflict
		}
		at, err := transactionTime(tx)
		if err != nil {
			return err
		}
		// 复用精确旧指纹升级，不把此次目录修复作为其他授权漂移的许可。
		if len(plan.Candidates) > 0 {
			if err := applyRootRepairHistory(tx, at, plan.History); err != nil {
				return err
			}
			if err := verifyRootRepairStreams(tx); err != nil {
				return err
			}
		}
		// 仅此显式维护事务已完成限定 Root 对账，禁止外层普通 Commit
		// 再对普通成员进行全量漂移重算。此函数拒绝嵌套在其他源写事务中。
		coordinated.dirty = false
		plan.Applied, plan.Repaired, plan.Operator, plan.AppliedAt = true, len(plan.Candidates), strings.TrimSpace(in.Operator), &at
		return nil
	})
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func inspectRootCatalog(db *gorm.DB, lock bool) (*RootCatalogRepairPlan, error) {
	var root im.Role
	query := db.Where("scope=? AND tenant_uuid=? AND code=?", "system", "", "root")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Take(&root).Error; err != nil {
		return nil, err
	}
	if !rootrole.IsManaged(root) || !root.Builtin || root.UUID == uuid.Nil {
		return nil, ErrRootRepairBlocked
	}
	var links []rootCatalogReference
	err := db.Table((&im.RolePermission{}).TableName()+" rp").
		Select("rp.*, p.permission_uuid AS canonical_permission_uuid, p.meta AS permission_meta, p.effect AS permission_effect, p.status AS permission_status, p.deleted_at IS NOT NULL AS permission_deleted, EXISTS(SELECT 1 FROM "+(&im.Role{}).TableName()+" owner WHERE owner.uuid=rp.role_uuid) AS role_uuid_exists").
		Joins("LEFT JOIN "+(&im.Permission{}).TableName()+" p ON p.id=rp.permission_id").
		Where("rp.role_id=? OR rp.role_uuid=?", root.ID, root.UUID).
		Order("rp.role_id, rp.permission_id").Scan(&links).Error
	if err != nil {
		return nil, err
	}
	plan := &RootCatalogRepairPlan{Contract: RootCatalogRepairContract, RootRoleID: root.ID, RootUUID: root.UUID, TotalLinks: len(links), Candidates: []RootCatalogRepairLink{}, Blocked: []RootCatalogRepairLink{}}
	for _, link := range links {
		item := RootCatalogRepairLink{PermissionID: link.PermissionID, PermissionUUID: link.PermissionUUID, OldRoleUUID: link.RoleUUID, CreatedAt: link.CreatedAt}
		switch {
		case link.RoleID != root.ID:
			item.Reason = "ROLE_NUMERIC_ID_CONFLICT"
		case link.CanonicalPermissionUUID == uuid.Nil || link.PermissionUUID != link.CanonicalPermissionUUID:
			item.Reason = "PERMISSION_UUID_CONFLICT"
		case link.RoleUUID == root.UUID:
			continue
		case link.RoleUUID == uuid.Nil || link.RoleUUIDExists:
			item.Reason = "ROLE_UUID_OWNERSHIP_CONFLICT"
		default:
			item.Reason = "DANGLING_SYSTEM_ROOT_ROLE_UUID"
			plan.Candidates = append(plan.Candidates, item)
			continue
		}
		plan.Blocked = append(plan.Blocked, item)
	}
	plan.History, plan.HistoryBlocked, err = inspectRootRepairHistory(db, plan)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(struct {
		Contract       string
		Root           im.Role
		Links          []rootCatalogReference
		History        []RootCatalogRepairHistory
		HistoryBlocked []RootCatalogRepairHistory
	}{RootCatalogRepairContract, root, links, plan.History, plan.HistoryBlocked})
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(raw)
	plan.Fingerprint = hex.EncodeToString(hash[:])
	return plan, nil
}

func verifyRootRepairStreams(db *gorm.DB) error {
	var tracks []im.AuthorizationTracking
	if err := db.Where("status=?", "ready").Find(&tracks).Error; err != nil {
		return err
	}
	for _, track := range tracks {
		facts, err := ar.NewRepository(db).Load(db.Statement.Context, track.TenantUUID, track.PluginID)
		if err != nil {
			return err
		}
		var streams []im.AuthorizationStream
		if err := db.Where("tenant_uuid=? AND plugin_id=? AND permission_code=?", track.TenantUUID, track.PluginID, track.PermissionCode).Find(&streams).Error; err != nil {
			return err
		}
		members := make(map[string]im.Member, len(facts.Members))
		for _, member := range facts.Members {
			members[member.UUID.String()] = member
		}
		for _, stream := range streams {
			member, exists := members[stream.MemberUUID]
			if !exists {
				continue
			}
			sources, root := MemberSources(facts, member, track.PermissionCode)
			if root && stream.FactsHash != Digest([]any{sources, root}) {
				return errors.New("IAM_ROOT_CATALOG_REPAIR_HISTORY_CONFLICT")
			}
		}
	}
	return nil
}
