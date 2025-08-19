package main

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/config"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"log"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
)

func ctx() context.Context { return context.Background() }

func ensureSystemAndRoot(db *gorm.DB) error {
	tenantRepo := infraiam.NewTenantRepository(db)
	roleRepo := infraiam.NewRoleRepository(db)
	userRepo := infraiam.NewUserRepository(db)
	credRepo := infraiam.NewCredentialRepository(db) // 下面给出实现
	urRepo := infraiam.NewUserRoleRepository(db)

	// 1) system tenant
	sysTenant, err := tenantRepo.EnsureSystemTenant(ctx())
	if err != nil {
		return err
	}

	// 2) tenant 默认角色
	if err := roleRepo.EnsureDefaultRoles(ctx(), sysTenant.ID); err != nil {
		return err
	}

	// 3) root 用户存在性
	u, err := userRepo.FindByUsername(ctx(), sysTenant.ID, "root")
	if err == nil && u != nil {
		fmt.Println("[seed] root already exists")
		return nil
	}

	// 4) 创建 root 用户 + 密码凭证
	root := &model.User{
		TenantID: sysTenant.ID,
		Username: "root",
		Status:   1,
	}
	if _, err = userRepo.Create(ctx(), root); err != nil {
		return err
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte("ChangeMe_123!"), bcrypt.DefaultCost)
	if err := credRepo.Create(ctx(), &model.Credential{
		UserID:     root.ID,
		Provider:   "password",
		Identifier: "root",
		SecretHash: string(hash),
		IsPrimary:  true,
	}); err != nil {
		return err
	}

	// 5) 绑定管理员角色
	if err := urRepo.AssignRolesByCodes(ctx(), sysTenant.ID, root.ID, "role_admin"); err != nil {
		return err
	}

	fmt.Printf("[seed] root created. tenant=system username=root password=ChangeMe_123!\n")
	return nil
}

func SeedCoreX(ctx context.Context, db *gorm.DB, cfg *config.Config) error {
	db, err := database.Connect(cfg.Database)
	if err != nil {
		//log.Fatal(err)
		return err
	}

	if err := ensureSystemAndRoot(db); err != nil {
		//log.Fatal(err)
		return err
	}
	log.Println("seed ok")

	return nil
}
