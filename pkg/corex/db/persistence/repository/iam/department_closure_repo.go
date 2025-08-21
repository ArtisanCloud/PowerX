// pkg/corex/db/persistence/repository/iam/department_closure_repo.go
package iam

import (
	"context"

	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
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

// EnsureSelfEdge: 插入 self 边 (ancestor=descendant, depth=0)
func (r *DepartmentClosureRepository) EnsureSelfEdge(ctx context.Context, tenantID, id uint64) error {
	t := (&dbm.DepartmentClosure{}).GetTableName(true)
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO `+t+` (tenant_id, ancestor_id, descendant_id, depth)
		 VALUES (?,?,?,0) ON CONFLICT DO NOTHING`,
		tenantID, id, id,
	).Error
}

// InheritFromParent: 让 child 继承 parent 的所有祖先（depth+1），并补充 parent→child（1）
func (r *DepartmentClosureRepository) InheritFromParent(ctx context.Context, tenantID, parentID, childID uint64) error {
	t := (&dbm.DepartmentClosure{}).GetTableName(true)
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO `+t+` (tenant_id, ancestor_id, descendant_id, depth)
		SELECT tenant_id, ancestor_id, ?, depth+1
		FROM `+t+` WHERE tenant_id=? AND descendant_id=?
		UNION ALL
		SELECT ?, ?, ?, 1
		ON CONFLICT DO NOTHING`,
		childID, tenantID, parentID,
		tenantID, parentID, childID,
	).Error
}

// RebuildSubtree: 重建某子树（通常在 Move 之后）
func (r *DepartmentClosureRepository) RebuildSubtree(ctx context.Context, tenantID, subtreeRootID uint64) error {
	tClosure := (&dbm.DepartmentClosure{}).GetTableName(true)
	tDept := (&dbm.Department{}).GetTableName(true)

	// 先删除子树所有节点的闭包边
	if err := r.db.WithContext(ctx).Exec(`
		DELETE FROM `+tClosure+` WHERE tenant_id=? AND descendant_id IN (
		  SELECT id FROM `+tDept+` WHERE tenant_id=? AND path LIKE (
		    SELECT CONCAT(path, id::text, '/') FROM `+tDept+` WHERE tenant_id=? AND id=?
		  ) || '%'
		)`, tenantID, tenantID, tenantID, subtreeRootID).Error; err != nil {
		return err
	}

	// 重新为子树每个节点插入 self 边 + 继承新父系（外部循环在 Service 做）
	return nil
}
