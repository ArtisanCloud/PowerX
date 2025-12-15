// pkg/cmd/database/seed/core.go
package seed

import (
	"errors"
	"fmt"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"

	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

func SeedRoot(db *gorm.DB) error {

	// 2) IAM 基础权限
	if err := SeedSystemPermissions(db); err != nil {
		return fmt.Errorf("seed system permissions: %w", err)
	}

	if err := SeedSwaggerPermissions(db, "./backend/api/openapi/swagger.json"); err != nil {
		return fmt.Errorf("seed swagger permissions: %w", err)
	}

	// 3) 确保 system 租户存在
	const tenantKey = "system"
	const orgName = "System"
	tenRepo := tenantrepo.NewTenantRepository(db)
	ten, err := tenRepo.EnsureByKey(seedCtx(), tenantKey, orgName, dbm.TenantPlanFree, dbm.TenantTypeSystem)
	if err != nil {
		return fmt.Errorf("ensure tenant(%s): %w", tenantKey, err)
	}

	tenantUUID := ten.UUID.String()

	// 4) 为该租户完成内置角色与授权（root(system) & tenant_admin(tenant)）
	if err := SeedBuiltInRolesAndGrants(db, tenantUUID); err != nil {
		return fmt.Errorf("seed built-in roles and grants: %w", err)
	}

	// 5) 为该租户确保默认角色，并授予基线权限（admin=全量，user=read）
	roleRepo := infraiam.NewRoleRepository(db)
	if err := roleRepo.EnsureDefaultRoles(seedCtx(), tenantUUID); err != nil {
		return fmt.Errorf("ensure default roles: %w", err)
	}
	if err := SeedGrantDefaultRolesForTenant(db, tenantUUID); err != nil {
		return fmt.Errorf("grant defaults for tenant %s: %w", tenantUUID, err)
	}

	// 6) 确保 root 用户与凭证
	const rootUserName = "root"
	const rootIdentifier = "root"
	rootPassword := envOrDefault("POWERX_ROOT_PASSWORD", "root")

	userRepo := infraiam.NewUserRepository(db)
	memberRepo := infraiam.NewMemberRepository(db)
	credRepo := infraiam.NewCredentialRepository(db)
	rbRepo := infraiam.NewRoleBindingRepository(db)

	cred, err := credRepo.FindByProviderIdentifier(seedCtx(), "password", rootIdentifier)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("find root credential: %w", err)
	}

	var userID uint64
	if cred != nil {
		userID = cred.UserID
	} else {
		u := &model.User{
			DisplayName: "root",
			Phone:       "13800000000",
			Email:       "tech@artisan-cloud.com",
			Status:      model.UserStatusActive,
			IsRoot:      true,
		}
		if _, err = userRepo.Create(seedCtx(), u); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		userID = u.ID

		hash, err := bcrypt.GenerateFromPassword([]byte(rootPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
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

	// 7) 在 system 租户确保 root 成员
	var memberID uint64
	mem, err := memberRepo.FindByTenantAndUser(seedCtx(), tenantUUID, userID)

	// 只有“真正的错误”才返回；ErrRecordNotFound 或 (nil,nil) 都进入创建分支
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("find member: %w", err)
	}

	if mem == nil || errors.Is(err, gorm.ErrRecordNotFound) {
		m := &model.Member{
			TenantUUID:  tenantUUID,
			UserID:      userID,
			Username:    rootUserName,
			DisplayName: "root",
			Status:      1,
		}
		if _, err := memberRepo.Create(seedCtx(), m); err != nil {
			return fmt.Errorf("create member: %w", err)
		}
		memberID = m.ID
	} else {
		memberID = mem.ID
	}

	// 8) 绑定 tenant 的 role_admin 到该成员（subject_type=MEMBER）
	adminRole, err := roleRepo.FindByCode(seedCtx(), "tenant", &tenantUUID, "role_admin")
	if err != nil {
		return fmt.Errorf("find role_admin: %w", err)
	}
	if err := rbRepo.Create(seedCtx(), &model.RoleBinding{
		TenantUUID:  tenantUUID,
		RoleID:      adminRole.ID,
		SubjectType: model.SubMember, // 你模型里定义的常量
		SubjectID:   memberID,
	}); err != nil {
		return fmt.Errorf("bind role_admin to root member: %w", err)
	}

	fmt.Printf("[seed] root ready. tenant=%s username=%s password=%s\n", tenantKey, rootUserName, rootPassword)
	return nil
}
