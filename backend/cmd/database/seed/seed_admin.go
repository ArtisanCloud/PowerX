// pkg/cmd/database/seed/core.go
package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apikeypermissions "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
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

	if err := SeedSwaggerPermissions(db, resolveSwaggerPath()); err != nil {
		return fmt.Errorf("seed swagger permissions: %w", err)
	}
	if err := apikeypermissions.EnsureTemplatePermissions(seedCtx(), infraiam.NewPermissionRepository(db)); err != nil {
		return fmt.Errorf("seed api key permissions: %w", err)
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
	if _, _, err := apikeypermissions.EnsureTenantDefaultProfile(seedCtx(), db, tenantUUID, nil); err != nil {
		return fmt.Errorf("ensure default api key profile for tenant(%s): %w", tenantUUID, err)
	}

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

	// 6) 确保 root 用户与凭证（仅从 setup 草稿文件或内置默认读取，不使用环境变量）
	rootUserName := "root"
	rootIdentifier := "root"
	rootDisplayName := "root"
	rootEmail := "tech@artisan-cloud.com"
	rootPhone := "13800000000"
	rootPassword := "root"

	setupAdmin, hasSetupDraft, hasSetupAdminPassword := loadSetupAdminFromDraft()
	if hasSetupAdminPassword {
		if v := strings.TrimSpace(setupAdmin.Username); v != "" {
			rootUserName = v
		}
		if v := strings.TrimSpace(setupAdmin.DisplayName); v != "" {
			rootDisplayName = v
		}
		if v := strings.ToLower(strings.TrimSpace(setupAdmin.Email)); v != "" {
			rootEmail = v
		}
		if v := strings.TrimSpace(setupAdmin.Phone); v != "" {
			rootPhone = v
		}
		rootPassword = strings.TrimSpace(setupAdmin.Password)
		rootIdentifier = strings.ToLower(strings.TrimSpace(rootEmail))
		if rootIdentifier == "" {
			rootIdentifier = strings.TrimSpace(rootPhone)
		}
		if rootIdentifier == "" {
			rootIdentifier = strings.TrimSpace(rootUserName)
		}
		if rootIdentifier == "" {
			rootIdentifier = "root"
		}
	} else if hasSetupDraft {
		// setup 进行中但尚未提供管理员密码：不提前写入默认 root。
		fmt.Println("[seed] skip root identity before setup admin is confirmed")
		return nil
	}

	userRepo := infraiam.NewUserRepository(db)
	memberRepo := infraiam.NewMemberRepository(db)
	credRepo := infraiam.NewCredentialRepository(db)
	rbRepo := infraiam.NewRoleBindingRepository(db)

	var userID uint64
	cred, err := credRepo.FindByProviderIdentifier(seedCtx(), "password", rootIdentifier)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("find root credential by identifier: %w", err)
	}
	if cred != nil {
		userID = cred.UserID
	}
	if userID == 0 {
		legacyCred, legacyErr := credRepo.FindByProviderIdentifier(seedCtx(), "password", "root")
		if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find legacy root credential: %w", legacyErr)
		}
		if legacyCred != nil {
			userID = legacyCred.UserID
		}
	}
	if userID == 0 && rootEmail != "" {
		if u, findErr := userRepo.FindByEmail(seedCtx(), rootEmail); findErr == nil && u != nil {
			userID = u.ID
		}
	}
	if userID == 0 && rootPhone != "" {
		if u, findErr := userRepo.FindByPhone(seedCtx(), rootPhone); findErr == nil && u != nil {
			userID = u.ID
		}
	}
	if userID == 0 {
		u := &model.User{
			DisplayName: rootDisplayName,
			Phone:       rootPhone,
			Email:       rootEmail,
			Status:      model.UserStatusActive,
			IsRoot:      true,
		}
		if _, err = userRepo.Create(seedCtx(), u); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		userID = u.ID
	} else {
		updates := map[string]any{
			"display_name": rootDisplayName,
			"is_root":      true,
		}
		if rootEmail != "" {
			updates["email"] = rootEmail
		}
		if rootPhone != "" {
			updates["phone"] = rootPhone
		}
		if err := db.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update root user: %w", err)
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(rootPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if cred != nil && cred.UserID != userID {
		return fmt.Errorf("root identifier already bound to another user: %s", rootIdentifier)
	}
	if err := credRepo.Upsert(seedCtx(), &model.Credential{
		UserID:     userID,
		Provider:   "password",
		Identifier: rootIdentifier,
		SecretHash: string(hash),
		IsPrimary:  true,
	}, "user_id", "secret_hash", "is_primary"); err != nil {
		return fmt.Errorf("upsert credential: %w", err)
	}

	// 兼容：如果 rootIdentifier 不是 root，再补一条 legacy root 标识，避免旧脚本/文档登录失败。
	if rootIdentifier != "root" {
		if legacyCred, legacyErr := credRepo.FindByProviderIdentifier(seedCtx(), "password", "root"); legacyErr == nil {
			if legacyCred.UserID != userID {
				return fmt.Errorf("legacy root identifier already bound to another user")
			}
		} else if errors.Is(legacyErr, gorm.ErrRecordNotFound) {
			if err := credRepo.Create(seedCtx(), &model.Credential{
				UserID:     userID,
				Provider:   "password",
				Identifier: "root",
				SecretHash: string(hash),
				IsPrimary:  false,
			}); err != nil {
				return fmt.Errorf("create legacy root credential: %w", err)
			}
		} else {
			return fmt.Errorf("find legacy root credential: %w", legacyErr)
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
			DisplayName: rootDisplayName,
			Status:      1,
		}
		if _, err := memberRepo.Create(seedCtx(), m); err != nil {
			return fmt.Errorf("create member: %w", err)
		}
		memberID = m.ID
	} else {
		memberID = mem.ID
		memberUpdates := map[string]any{
			"username":     rootUserName,
			"display_name": rootDisplayName,
		}
		if err := db.Model(&model.Member{}).Where("id = ?", memberID).Updates(memberUpdates).Error; err != nil {
			return fmt.Errorf("update root member: %w", err)
		}
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

	fmt.Printf("[seed] root ready. tenant=%s username=%s identifier=%s password=%s\n", tenantKey, rootUserName, rootIdentifier, rootPassword)
	return nil
}

type setupDraftAdminConfig struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Phone       string `json:"phone"`
}

type setupDraftPayload struct {
	Admin setupDraftAdminConfig `json:"admin"`
}

func loadSetupAdminFromDraft() (setupDraftAdminConfig, bool, bool) {
	paths := []string{
		filepath.Join("etc", "setup.wizard.config.json"),
		filepath.Join("backend", "etc", "setup.wizard.config.json"),
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var payload setupDraftPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return setupDraftAdminConfig{}, true, false
		}
		admin := payload.Admin
		hasPassword := strings.TrimSpace(admin.Password) != ""
		return admin, true, hasPassword
	}
	return setupDraftAdminConfig{}, false, false
}
