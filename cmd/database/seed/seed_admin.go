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

func SeedRoot(db *gorm.DB) error {
	if err := SeedSystemBuiltinRoles(db); err != nil {
		return fmt.Errorf("seed system builtin roles: %w", err)
	}
	if err := SeedSystemPermissions(db); err != nil {
		return fmt.Errorf("seed system permissions: %w", err)
	}

	const tenantKey = "system"
	const orgName = "System"
	const rootUserName = "root"
	const rootIdentifier = "root"
	rootPassword := envOrDefault("POWERX_ROOT_PASSWORD", "root")

	tenantRepo := tenantRepo.NewTenantRepository(db)
	roleRepo := infraiam.NewRoleRepository(db)
	userRepo := infraiam.NewUserRepository(db)
	memberRepo := infraiam.NewMemberRepository(db)
	credRepo := infraiam.NewCredentialRepository(db)
	rbRepo := infraiam.NewRoleBindingRepository(db) // ★ 新增：角色绑定仓储

	// 1) 租户
	ten, err := tenantRepo.EnsureByKey(seedCtx(), tenantKey, orgName)
	if err != nil {
		return fmt.Errorf("ensure tenant: %w", err)
	}

	// 2) 默认角色（role_admin / role_user）
	if err := roleRepo.EnsureDefaultRoles(seedCtx(), ten.ID); err != nil {
		return fmt.Errorf("ensure roles: %w", err)
	}
	if err := SeedGrantDefaultRolesForTenant(db, ten.ID); err != nil {
		return fmt.Errorf("grant defaults for tenant %d: %w", ten.ID, err)
	}

	// 3) root 凭证
	cred, err := credRepo.FindByProviderIdentifier(seedCtx(), "password", rootIdentifier)

	var userID uint64
	if err == nil && cred != nil {
		userID = cred.UserID
	} else {
		u := &model.User{DisplayName: "root", Status: 1, IsRoot: true}
		if _, err = userRepo.Create(seedCtx(), u); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		userID = u.ID

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

	// 4) 在 system 租户创建 root 成员
	var memberID uint64
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
		memberID = m.ID
	} else {
		// 若已存在，可以再查一次拿 ID（或在上面 Find 里返回）
		exist, _ := memberRepo.FindByTenantAndUser(seedCtx(), ten.ID, userID)
		memberID = exist.ID
	}

	// 5) 绑定 role_admin 到该成员（走 iam_role_binding，subject_type=MEMBER）
	adminRole, err := roleRepo.FindByCode(seedCtx(), "tenant", &ten.ID, "role_admin")
	if err != nil {
		return fmt.Errorf("find role_admin: %w", err)
	}
	if err := rbRepo.Create(seedCtx(), &model.RoleBinding{
		TenantID:    ten.ID,
		RoleID:      adminRole.ID,
		SubjectType: model.SubMember, // ← 你模型里定义的常量
		SubjectID:   memberID,
		// DataScope 等字段用默认（TENANT）即可；需要可在此自定义
	}); err != nil {
		return fmt.Errorf("bind role_admin to root member: %w", err)
	}

	fmt.Printf("[seed] root ready. tenant=%s username=%s password=%s\n", tenantKey, rootUserName, rootPassword)
	return nil
}
