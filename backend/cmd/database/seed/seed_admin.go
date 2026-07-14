// pkg/cmd/database/seed/core.go
package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	iamservice "github.com/ArtisanCloud/PowerX/internal/service/iam"
	apikeypermissions "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"

	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func SeedRoot(db *gorm.DB) error {

	// 2) IAM 基础权限
	if err := SeedSystemPermissions(db); err != nil {
		return fmt.Errorf("seed system permissions: %w", err)
	}

	if err := SeedSwaggerPermissions(db, resolveSwaggerPath()); err != nil {
		return fmt.Errorf("seed swagger permissions: %w", err)
	}
	if err := iamservice.EnsureOpsPermissions(seedCtx(), permissionRegistrar{repo: infraiam.NewPermissionRepository(db)}); err != nil {
		return fmt.Errorf("seed ops permissions: %w", err)
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

	// 6) 确保 root 用户与凭证（从 setup 运行时草稿文件或内置默认读取）
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
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] skip root identity before setup admin is confirmed")
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
	// 如果 legacy 标识已存在，必须同步 hash，否则 setup 密码更新后 root/root 会残留。
	if rootIdentifier != "root" {
		if legacyCred, legacyErr := credRepo.FindByProviderIdentifier(seedCtx(), "password", "root"); legacyErr == nil {
			if legacyCred.UserID != userID {
				return fmt.Errorf("legacy root identifier already bound to another user")
			}
			if err := credRepo.Upsert(seedCtx(), &model.Credential{
				UserID:     userID,
				Provider:   "password",
				Identifier: "root",
				SecretHash: string(hash),
				IsPrimary:  false,
			}, "user_id", "secret_hash", "is_primary"); err != nil {
				return fmt.Errorf("sync legacy root credential: %w", err)
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

	// 7) 在 system 租户确保 root 成员。
	// SaaS IAM 语义要求保留 root user + system tenant member 作为平台身份锚点；
	// 不要在 seed 中把 root 自动补进任何业务租户。
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

	if err := SeedDemoReadonlyAccount(db); err != nil {
		return fmt.Errorf("seed demo readonly account: %w", err)
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] root ready. tenant=%s username=%s identifier=%s password_source=%s", tenantKey, rootUserName, rootIdentifier, rootPasswordSource(hasSetupAdminPassword))
	return nil
}

type permissionRegistrar struct {
	repo *infraiam.PermissionRepository
}

func (r permissionRegistrar) RegisterPermissions(ctx context.Context, rows []model.Permission) error {
	if r.repo == nil {
		return errors.New("permission repository is nil")
	}
	return r.repo.UpsertBatch(ctx, rows)
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
	for _, p := range setupDraftCandidatePaths() {
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

func rootPasswordSource(fromSetupDraft bool) string {
	if fromSetupDraft {
		return "setup_draft"
	}
	return "built_in_default"
}

func setupDraftCandidatePaths() []string {
	const draftFile = "setup.wizard.config.json"
	paths := make([]string, 0, 8)
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		for _, existing := range paths {
			if existing == p {
				return
			}
		}
		paths = append(paths, p)
	}
	addDraftForConfigPath := func(configPath string) {
		configPath = strings.TrimSpace(configPath)
		if configPath == "" {
			return
		}
		if abs, err := filepath.Abs(configPath); err == nil {
			configPath = abs
		}
		add(filepath.Join(filepath.Dir(configPath), draftFile))
	}

	add(os.Getenv("POWERX_SETUP_DRAFT_PATH"))
	addDraftForConfigPath(os.Getenv("POWERX_SETUP_RUNTIME_CONFIG_PATH"))
	addDraftForConfigPath(os.Getenv("POWERX_CONFIG"))
	if root := strings.TrimSpace(os.Getenv("POWERX_RUNTIME_ROOT")); root != "" {
		add(filepath.Join(root, draftFile))
	}
	add(filepath.Join(string(filepath.Separator), "etc", "powerx", draftFile))
	add(filepath.Join("etc", draftFile))
	add(filepath.Join("backend", "etc", draftFile))
	return paths
}

func SeedDemoReadonlyAccount(db *gorm.DB) error {
	if !isTruthyEnv("POWERX_ENABLE_DEMO_ACCOUNT") {
		return nil
	}
	ctx := seedCtx()
	tenantKey := strings.TrimSpace(envOrDefault("POWERX_DEMO_TENANT_KEY", "demo"))
	tenantName := strings.TrimSpace(envOrDefault("POWERX_DEMO_TENANT_NAME", "Demo Space"))
	username := strings.TrimSpace(envOrDefault("POWERX_DEMO_USERNAME", "demo"))
	displayName := strings.TrimSpace(envOrDefault("POWERX_DEMO_DISPLAY_NAME", "Demo User"))
	email := strings.ToLower(strings.TrimSpace(envOrDefault("POWERX_DEMO_EMAIL", "demo@powerx.local")))
	phone := strings.TrimSpace(envOrDefault("POWERX_DEMO_PHONE", ""))
	password := strings.TrimSpace(envOrDefault("POWERX_DEMO_PASSWORD", "demo123456"))
	if tenantKey == "" || tenantName == "" || username == "" || password == "" {
		return errors.New("invalid demo seed env: tenant key/name, username and password are required")
	}
	identifier := email
	if identifier == "" {
		identifier = strings.ToLower(username)
	}
	if identifier == "" {
		return errors.New("invalid demo seed env: identifier is empty")
	}

	tenRepo := tenantrepo.NewTenantRepository(db)
	ten, err := tenRepo.EnsureByKey(ctx, tenantKey, tenantName, dbm.TenantPlanFree, dbm.TenantTypePersonal)
	if err != nil {
		return fmt.Errorf("ensure demo tenant: %w", err)
	}
	tenantUUID := ten.UUID.String()
	roleRepo := infraiam.NewRoleRepository(db)
	if err := roleRepo.EnsureDefaultRoles(ctx, tenantUUID); err != nil {
		return fmt.Errorf("ensure default roles for demo tenant: %w", err)
	}
	if err := SeedGrantDefaultRolesForTenant(db, tenantUUID); err != nil {
		return fmt.Errorf("grant defaults for demo tenant: %w", err)
	}
	readonlyRole, err := roleRepo.FindByCode(ctx, "tenant", &tenantUUID, "role_readonly")
	if err != nil {
		return fmt.Errorf("find demo readonly role: %w", err)
	}

	userRepo := infraiam.NewUserRepository(db)
	memberRepo := infraiam.NewMemberRepository(db)
	credRepo := infraiam.NewCredentialRepository(db)

	var userID uint64
	cred, err := credRepo.FindByProviderIdentifier(ctx, "password", identifier)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("find demo credential: %w", err)
	}
	if cred != nil {
		userID = cred.UserID
	}
	if userID == 0 && email != "" {
		if u, findErr := userRepo.FindByEmail(ctx, email); findErr == nil && u != nil {
			userID = u.ID
		}
	}
	if userID == 0 && phone != "" {
		if u, findErr := userRepo.FindByPhone(ctx, phone); findErr == nil && u != nil {
			userID = u.ID
		}
	}
	if userID == 0 {
		u := &model.User{
			DisplayName: displayName,
			Phone:       phone,
			Email:       email,
			Status:      model.UserStatusActive,
			IsRoot:      false,
		}
		if _, err = userRepo.Create(ctx, u); err != nil {
			return fmt.Errorf("create demo user: %w", err)
		}
		userID = u.ID
	} else {
		updates := map[string]any{
			"display_name": displayName,
			"is_root":      false,
			"status":       model.UserStatusActive,
		}
		if email != "" {
			updates["email"] = email
		}
		if phone != "" {
			updates["phone"] = phone
		}
		if err := db.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update demo user: %w", err)
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash demo password: %w", err)
	}
	if cred != nil && cred.UserID != userID {
		return fmt.Errorf("demo identifier already bound to another user: %s", identifier)
	}
	if err := credRepo.Upsert(ctx, &model.Credential{
		UserID:     userID,
		Provider:   "password",
		Identifier: identifier,
		SecretHash: string(hash),
		IsPrimary:  true,
	}, "user_id", "secret_hash", "is_primary"); err != nil {
		return fmt.Errorf("upsert demo credential: %w", err)
	}
	if strings.ToLower(username) != identifier {
		if err := credRepo.Upsert(ctx, &model.Credential{
			UserID:     userID,
			Provider:   "password",
			Identifier: strings.ToLower(username),
			SecretHash: string(hash),
			IsPrimary:  false,
		}, "user_id", "secret_hash", "is_primary"); err != nil {
			return fmt.Errorf("upsert demo username credential: %w", err)
		}
	}

	member, err := memberRepo.FindByTenantAndUser(ctx, tenantUUID, userID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("find demo member: %w", err)
	}
	var memberID uint64
	if member == nil {
		m := &model.Member{
			TenantUUID:  tenantUUID,
			UserID:      userID,
			Username:    username,
			DisplayName: displayName,
			Status:      1,
		}
		if _, err := memberRepo.Create(ctx, m); err != nil {
			return fmt.Errorf("create demo member: %w", err)
		}
		memberID = m.ID
	} else {
		memberID = member.ID
		if err := db.Model(&model.Member{}).Where("id = ?", memberID).Updates(map[string]any{
			"username":     username,
			"display_name": displayName,
			"status":       1,
		}).Error; err != nil {
			return fmt.Errorf("update demo member: %w", err)
		}
	}

	if err := db.WithContext(ctx).
		Where("tenant_uuid = ? AND subject_type = ? AND subject_id = ?", tenantUUID, model.SubMember, memberID).
		Delete(&model.RoleBinding{}).Error; err != nil {
		return fmt.Errorf("clear demo member role bindings: %w", err)
	}
	if err := infraiam.NewRoleBindingRepository(db).Create(ctx, &model.RoleBinding{
		TenantUUID:  tenantUUID,
		RoleID:      readonlyRole.ID,
		SubjectType: model.SubMember,
		SubjectID:   memberID,
	}); err != nil {
		return fmt.Errorf("bind demo readonly role: %w", err)
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] demo readonly ready. tenant=%s username=%s identifier=%s password=%s", tenantKey, username, identifier, password)
	return nil
}

func isTruthyEnv(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
