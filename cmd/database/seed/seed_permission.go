package seed

import (
	"fmt"

	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

// SeedSystemPermissions：把核心模块(IAM)的一批常用权限写入 iam_permission（幂等）
func SeedSystemPermissions(db *gorm.DB) error {
	pr := infraiam.NewPermissionRepository(db)

	perms := []dbm.Permission{
		// IAM / Role
		{Plugin: "iam", Resource: "role", Action: "read"},
		{Plugin: "iam", Resource: "role", Action: "write"},
		{Plugin: "iam", Resource: "role", Action: "delete"},
		{Plugin: "iam", Resource: "role", Action: "bind"},
		// IAM / User
		{Plugin: "iam", Resource: "user", Action: "read"},
		{Plugin: "iam", Resource: "user", Action: "write"},
		{Plugin: "iam", Resource: "user", Action: "delete"},
		// IAM / Department
		{Plugin: "iam", Resource: "department", Action: "read"},
		{Plugin: "iam", Resource: "department", Action: "write"},
		{Plugin: "iam", Resource: "department", Action: "delete"},
		// IAM / Permission（只读）
		{Plugin: "iam", Resource: "permission", Action: "read"},
	}

	// 你仓储里已有 UpsertBatch：幂等插入/更新
	if err := pr.UpsertBatch(seedCtx(), perms); err != nil {
		return fmt.Errorf("upsert system permissions: %w", err)
	}
	fmt.Printf("[seed] system permissions ready: %d\n", len(perms))
	return nil
}
