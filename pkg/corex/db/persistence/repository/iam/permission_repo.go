package iam

import (
	"context"
	"errors"
	"fmt"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

// PermissionRepository：基础 CRUD + 批量 Upsert + 用户权限检查
type PermissionRepository struct {
	*repository.BaseRepository[dbm.Permission]
	db *gorm.DB
}

func NewPermissionRepository(db *gorm.DB) *PermissionRepository {
	return &PermissionRepository{
		BaseRepository: repository.NewBaseRepository[dbm.Permission](db),
		db:             db,
	}
}

// UpsertBatch 以 (plugin,resource,action) 为唯一键幂等写入
func (r *PermissionRepository) UpsertBatch(ctx context.Context, rows []dbm.Permission) error {
	if len(rows) == 0 {
		return nil
	}
	// BaseRepository 的 UpsertBatch 接收 []*T
	ptrs := make([]*dbm.Permission, 0, len(rows))
	for i := range rows {
		ptrs = append(ptrs, &rows[i])
	}
	_, err := r.BaseRepository.UpsertBatch(ctx, ptrs, []clause.Column{
		{Name: "plugin"}, {Name: "resource"}, {Name: "action"},
	})
	return err
}

// List 原始列表 + 统计（用于服务层分页）
func (r *PermissionRepository) List(ctx context.Context, filter map[string]string, offset, limit int, sort string) (list []dbm.Permission, total int64, err error) {
	q := r.db.WithContext(ctx).Model(&dbm.Permission{})
	if v := strings.TrimSpace(filter["plugin"]); v != "" {
		q = q.Where("plugin = ?", v)
	}
	if v := strings.TrimSpace(filter["resource"]); v != "" {
		q = q.Where("resource = ?", v)
	}
	if v := strings.TrimSpace(filter["module"]); v != "" {
		q = q.Where("meta->>'module' = ?", v)
	}
	if v := strings.TrimSpace(filter["type"]); v != "" {
		q = q.Where("meta->>'type' = ?", v)
	}
	if v := strings.TrimSpace(filter["status"]); v != "" {
		q = q.Where("status = ?", v)
	}
	if kw := strings.TrimSpace(filter["keyword"]); kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		q = q.Where(
			"LOWER(plugin) LIKE ? OR LOWER(resource) LIKE ? OR LOWER(action) LIKE ? OR LOWER(description) LIKE ? OR LOWER(meta->>'label') LIKE ?",
			like, like, like, like, like,
		)
	}
	if err = q.Count(&total).Error; err != nil {
		return
	}
	if sort == "" {
		sort = "plugin ASC, resource ASC, action ASC"
	}
	err = q.Order(sort).Offset(offset).Limit(limit).Find(&list).Error
	return
}

// —— 同步差异结构 —— //
type DiffItem struct {
	Kind   string          `json:"kind"` // add|update|deprecate|nochange
	Before *dbm.Permission `json:"before,omitempty"`
	After  *dbm.Permission `json:"after,omitempty"`
}

type SyncResult struct {
	Source     string     `json:"source"`
	Introduced string     `json:"introduced"`
	Added      int        `json:"added"`
	Updated    int        `json:"updated"`
	Deprecated int        `json:"deprecated"`
	NoChange   int        `json:"no_change"`
	Diff       []DiffItem `json:"diff"`
}

// Sync 执行同步；若 dryRun=true 只返回 diff，不落库
func (r *PermissionRepository) Sync(ctx context.Context, source, introduced string, incoming []dbm.Permission, dryRun bool) (SyncResult, error) {
	if source == "" {
		return SyncResult{}, errors.New("source required")
	}

	// 1) 取出该 source 下的现有条目
	var exist []dbm.Permission
	if err := r.db.WithContext(ctx).
		Where("source = ? OR (source = '' AND ? = 'core')", source, source). // 兼容老数据
		Find(&exist).Error; err != nil {
		return SyncResult{}, err
	}

	key := func(p dbm.Permission) string { return fmt.Sprintf("%s|%s|%s", p.Plugin, p.Resource, p.Action) }
	exMap := map[string]dbm.Permission{}
	for _, e := range exist {
		exMap[key(e)] = e
	}

	inMap := map[string]dbm.Permission{}
	for _, it := range incoming {
		it.Source = source
		if it.Introduced == "" {
			it.Introduced = introduced
		}
		it.Status = dbm.PermissionStatusActive
		inMap[key(it)] = it
	}

	var diff []DiffItem
	var toUpsert []dbm.Permission
	now := time.Now().Unix()

	// 2) 新增/更新/nochange
	for k, it := range inMap {
		if old, ok := exMap[k]; ok {
			changed := (old.Effect != it.Effect) ||
				(strings.TrimSpace(old.Description) != strings.TrimSpace(it.Description)) ||
				(string(old.Meta) != string(it.Meta)) ||
				(old.Status != dbm.PermissionStatusActive)
			if changed {
				it.ID = old.ID
				diff = append(diff, DiffItem{Kind: "update", Before: &old, After: &it})
				toUpsert = append(toUpsert, it)
			} else {
				diff = append(diff, DiffItem{Kind: "nochange", Before: &old, After: &it})
			}
		} else {
			diff = append(diff, DiffItem{Kind: "add", After: &it})
			toUpsert = append(toUpsert, it)
		}
	}

	// 3) 废弃（不在 incoming 且来源相同）
	var toDeprecateIDs []uint64
	for k, old := range exMap {
		if _, ok := inMap[k]; !ok {
			cp := old
			cp.Status = dbm.PermissionStatusDeprecated
			ts := now
			cp.DeprecatedAt = &ts
			diff = append(diff, DiffItem{Kind: "deprecate", Before: &old, After: &cp})
			toDeprecateIDs = append(toDeprecateIDs, old.ID)
		}
	}

	// 统计
	res := SyncResult{Source: source, Introduced: introduced, Diff: diff}
	for _, d := range diff {
		switch d.Kind {
		case "add":
			res.Added++
		case "update":
			res.Updated++
		case "deprecate":
			res.Deprecated++
		case "nochange":
			res.NoChange++
		}
	}

	if dryRun {
		return res, nil
	}

	// 落库：1) upsert 新增/更新  2) 批量标记废弃
	if len(toUpsert) > 0 {
		if err := r.UpsertBatch(ctx, toUpsert); err != nil {
			return res, err
		}
	}
	if len(toDeprecateIDs) > 0 {
		if err := r.db.WithContext(ctx).Model(&dbm.Permission{}).
			Where("id IN ?", toDeprecateIDs).
			Updates(map[string]any{"status": dbm.PermissionStatusDeprecated, "deprecated_at": now}).Error; err != nil {
			return res, err
		}
	}
	return res, nil
}

// UserHasPermission 判断用户在租户下是否拥有某资源-动作
func (r *PermissionRepository) UserHasPermission(ctx context.Context, tenantID, userID uint64, resource, action string) (bool, error) {
	var cnt int64
	err := r.db.WithContext(ctx).
		Table((&dbm.Permission{}).GetTableName(true)+" AS p").
		Select("COUNT(1)").
		Joins("JOIN "+(&dbm.RolePermission{}).GetTableName(true)+" rp ON rp.permission_id = p.id").
		Joins("JOIN "+(&dbm.RoleBinding{}).GetTableName(true)+" ur ON ur.role_id = rp.role_id AND ur.tenant_id = ?", tenantID).
		Where("ur.user_id = ? AND p.resource = ? AND p.action = ?", userID, resource, action).
		Count(&cnt).Error
	return cnt > 0, err
}

// MemberHasPermissionViaBinding：成员在某租户下是否拥有 resource/action 权限
func (r *PermissionRepository) MemberHasPermissionViaBinding(
	ctx context.Context,
	tenantID, memberID uint64,
	resource, action string,
) (bool, error) {
	var cnt int64
	err := r.db.WithContext(ctx).
		Table((&dbm.Permission{}).GetTableName(true)+" AS p").
		Select("COUNT(1)").
		Joins("JOIN "+(&dbm.RolePermission{}).GetTableName(true)+" rp ON rp.permission_id = p.id").
		Joins("JOIN "+(&dbm.RoleBinding{}).GetTableName(true)+" rb ON rb.role_id = rp.role_id AND rb.tenant_id = ? AND rb.subject_type = ?", tenantID, dbm.SubMember).
		Where("rb.subject_id = ? AND p.resource = ? AND p.action = ?", memberID, resource, action).
		Count(&cnt).Error
	return cnt > 0, err
}
