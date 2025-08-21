// pkg/corex/db/persistence/model/iam/department_closure_gorm.go
package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type DepartmentClosure struct {
	TenantID     uint64 `gorm:"column:tenant_id;not null;index"`
	AncestorID   uint64 `gorm:"column:ancestor_id;not null;index"`
	DescendantID uint64 `gorm:"column:descendant_id;not null;index"`
	Depth        int16  `gorm:"column:depth;not null"` // 0=self, 1=parent, ...
}

// 唯一：一个租户内 (ancestor, descendant) 唯一
// gorm: uniqueIndex:uk_dept_closure,composite:(tenant_id,ancestor_id,descendant_id)

func (mdl *DepartmentClosure) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMDepartmentClosure
}

func (mdl *DepartmentClosure) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMDepartmentClosure
}
