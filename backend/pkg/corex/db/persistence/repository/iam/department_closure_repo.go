package iam

import (
	"context"

	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type DepartmentClosureRepository struct {
	*repository.BaseRepository[dbm.DepartmentClosure]
	db *gorm.DB
}

func NewDepartmentClosureRepository(db *gorm.DB) *DepartmentClosureRepository {
	return &DepartmentClosureRepository{
		BaseRepository: repository.NewBaseRepository[dbm.DepartmentClosure](db),
		db:             db,
	}
}

// ------- Tx 版本：务必在外部事务里调用 -------

// EnsureSelfEdgeTx: 插入 self 边 (ancestor=descendant, depth=0)
func (r *DepartmentClosureRepository) EnsureSelfEdgeTx(ctx context.Context, tx *gorm.DB, tenantUUID string, id uint64) error {
	t := (&dbm.DepartmentClosure{}).GetTableName(true)
	return tx.WithContext(ctx).Exec(
		`INSERT INTO `+t+` (tenant_uuid, ancestor_id, descendant_id, depth)
		 VALUES (?, ?, ?, 0)
		 ON CONFLICT DO NOTHING`,
		tenantUUID, id, id,
	).Error
}

// InheritFromParentTx: 让 child 继承 parent 的所有祖先（depth+1），并补充 parent→child（1）
func (r *DepartmentClosureRepository) InheritFromParentTx(ctx context.Context, tx *gorm.DB, tenantUUID string, parentID, childID uint64) error {
	t := (&dbm.DepartmentClosure{}).GetTableName(true)

	// ① 继承父亲的所有祖先 → child
	if err := tx.WithContext(ctx).Exec(`
		INSERT INTO `+t+` (tenant_uuid, ancestor_id, descendant_id, depth)
		SELECT tenant_uuid, ancestor_id, ?, depth + 1
		  FROM `+t+`
		 WHERE tenant_uuid = ? AND descendant_id = ?
		ON CONFLICT DO NOTHING`,
		childID, tenantUUID, parentID,
	).Error; err != nil {
		return err
	}

	// ② 增加 parent → child 的直接边（depth=1）
	return tx.WithContext(ctx).Exec(`
		INSERT INTO `+t+` (tenant_uuid, ancestor_id, descendant_id, depth)
		VALUES (?, ?, ?, 1)
		ON CONFLICT DO NOTHING`,
		tenantUUID, parentID, childID,
	).Error
}

// RebuildSubtreeTx: 重建某子树（通常在 Move 之后调用）
func (r *DepartmentClosureRepository) RebuildSubtreeTx(ctx context.Context, tx *gorm.DB, tenantUUID string, subtreeRootID uint64) error {
	tClosure := (&dbm.DepartmentClosure{}).GetTableName(true)
	tDept := (&dbm.Department{}).GetTableName(true)

	// 删除子树所有节点的闭包边
	if err := tx.WithContext(ctx).Exec(`
		DELETE FROM `+tClosure+` 
		 WHERE tenant_uuid = ?
		   AND descendant_id IN (
		        SELECT id FROM `+tDept+`
		         WHERE tenant_uuid = ?
		           AND path LIKE (
		               SELECT CONCAT(path, id::text, '/')
		                 FROM `+tDept+` 
		                WHERE tenant_uuid = ? AND id = ?
		           ) || '%'
		   )`,
		tenantUUID, tenantUUID, tenantUUID, subtreeRootID,
	).Error; err != nil {
		return err
	}
	return nil
}

// DeleteNodeTx：删除单节点相关闭包边（自环 + 所有关联）
func (r *DepartmentClosureRepository) DeleteNodeTx(
	ctx context.Context, tx *gorm.DB, tenantUUID string, id uint64,
) error {
	t := (&dbm.DepartmentClosure{}).GetTableName(true)
	return tx.WithContext(ctx).Exec(`
        DELETE FROM `+t+`
         WHERE tenant_uuid = ?
           AND (ancestor_id = ? OR descendant_id = ?)`,
		tenantUUID, id, id,
	).Error
}

// DeleteSubtreeTx：删除以 rootID 为根的子树所有闭包边
func (r *DepartmentClosureRepository) DeleteSubtreeTx(
	ctx context.Context, tx *gorm.DB, tenantUUID string, rootID uint64,
) error {
	tC := (&dbm.DepartmentClosure{}).GetTableName(true)
	tD := (&dbm.Department{}).GetTableName(true)

	// 通过 path 前缀找出子树 id 集合，删除 ancestor/descendant 命中的所有边
	return tx.WithContext(ctx).Exec(`
        DELETE FROM `+tC+`
         WHERE tenant_uuid = ?
           AND (
                ancestor_id IN (
                    SELECT id FROM `+tD+`
                     WHERE tenant_uuid = ?
                       AND path LIKE (
                           SELECT path FROM `+tD+`
                            WHERE tenant_uuid = ? AND id = ?
                       ) || '%'
                )
             OR descendant_id IN (
                    SELECT id FROM `+tD+`
                     WHERE tenant_uuid = ?
                       AND path LIKE (
                           SELECT path FROM `+tD+`
                            WHERE tenant_uuid = ? AND id = ?
                       ) || '%'
                )
           )`,
		tenantUUID, tenantUUID, tenantUUID, rootID, tenantUUID, tenantUUID, rootID,
	).Error
}
