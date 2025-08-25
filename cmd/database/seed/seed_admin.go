// pkg/cmd/database/seed/core.go
package seed

import (
	"fmt"
	tenantRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

// SeedRoot 仅做“最小种子”：system 租户 + root 管理员（root/root，可用 POWERX_ROOT_PASSWORD 覆盖）
func SeedRoot(db *gorm.DB) error {
	const tenantKey = "system"
	const orgName = "System"
	const rootUserName = "root"   // 成员在租户内的用户名
	const rootIdentifier = "root" // 凭证 identifier（你也可换成邮箱）

	rootPassword := envOrDefault("POWERX_ROOT_PASSWORD", "root")

	tenantRepo := tenantRepo.NewTenantRepository(db)
	roleRepo := infraiam.NewRoleRepository(db)
	userRepo := infraiam.NewUserRepository(db)       // 全局 User（不带 tenant）
	memberRepo := infraiam.NewMemberRepository(db)   // 租户成员
	credRepo := infraiam.NewCredentialRepository(db) // 全局 Credential
	mrRepo := infraiam.NewMemberRoleRepository(db)   // 成员-角色

	// 1) 确保 system 租户
	ten, err := tenantRepo.EnsureByKey(seedCtx(), tenantKey, orgName)
	if err != nil {
		return fmt.Errorf("ensure tenant: %w", err)
	}

	// 2) 确保默认角色（至少包含 role_admin / role_user）
	if err := roleRepo.EnsureDefaultRoles(seedCtx(), ten.ID); err != nil {
		return fmt.Errorf("ensure roles: %w", err)
	}

	// 3) 看是否已有 root 凭证（password/root）
	cred, err := credRepo.FindByProviderIdentifier(seedCtx(), "password", rootIdentifier)

	var userID uint64
	if err == nil && cred != nil {
		// 已存在，直接复用
		userID = cred.UserID
	} else {
		// 3.1 创建全局 User（最小字段；如需邮箱后续自行补充）
		u := &model.User{
			DisplayName: "root",
			Status:      1,
			IsRoot:      true,
		}
		if _, err = userRepo.Create(seedCtx(), u); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		userID = u.ID

		// 3.2 创建密码凭证（password/root）
		hash, _ := bcrypt.GenerateFromPassword([]byte(rootPassword), bcrypt.DefaultCost)
		if err := credRepo.Create(seedCtx(), &model.Credential{
			UserID:     userID,
			Provider:   "password",
			Identifier: rootIdentifier,
			SecretHash: string(hash),
			IsPrimary:  true,
		}); err != nil {
			return fmt.Errorf("create credential: %w", err)
		}
	}

	// 4) 确保 system 下存在一个 member（username=root）
	if _, err = memberRepo.FindByTenantAndUser(seedCtx(), ten.ID, userID); err != nil {
		m := &model.Member{
			TenantID:    ten.ID,
			UserID:      userID,
			Username:    rootUserName,
			DisplayName: "root",
			Status:      1,
		}
		if _, err = memberRepo.Create(seedCtx(), m); err != nil {
			return fmt.Errorf("create member: %w", err)
		}
		// 5) 绑定管理员角色（幂等）
		if err := mrRepo.AssignRolesByCodes(seedCtx(), ten.ID, m.ID, "role_admin"); err != nil {
			return fmt.Errorf("assign roles: %w", err)
		}
	}

	fmt.Printf("[seed] root ready. tenant=%s username=%s password=%s\n", tenantKey, rootUserName, rootPassword)
	return nil
}
