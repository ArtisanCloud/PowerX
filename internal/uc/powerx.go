package uc

import (
	"PowerX/internal/config"
	"context"
	"github.com/pkg/errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PowerXUseCase struct {
	db          *gorm.DB
	Auth        *AuthUseCase
	Employee    *EmployeeUseCase
	Department  *DepartmentUseCase
	Tag         *TagUseCase
	Contact     *ContactUseCase
	WeWork      *WeWorkUseCase
	SyncWeWork  *SyncWeWorkUseCase
	MetadataCtx *MetadataCtx
}

func NewPowerXUseCase(conf *config.Config) (uc *PowerXUseCase, clean func()) {
	// 启动数据库并测试连通性
	db, err := gorm.Open(postgres.Open(conf.PowerXDatabase.DSN), &gorm.Config{
		//Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		panic(errors.Wrap(err, "connect database failed"))
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic(errors.Wrap(err, "get sql db failed"))
	}
	err = sqlDB.Ping()
	if err != nil {
		panic(errors.Wrap(err, "ping database failed"))
	}

	uc = &PowerXUseCase{
		db: db,
	}
	// 加载子UseCase
	uc.MetadataCtx = newMetadataCtx()
	uc.Employee = newEmployeeUseCase(db)
	uc.Auth = newCasbinUseCase(db, uc.MetadataCtx, uc.Employee)
	uc.Department = newDepartmentUseCase(db)
	uc.Tag = newTagUseCase(db)
	uc.Contact = newContactUseCase(db)
	uc.WeWork = newWeWorkUseCase(conf)
	uc.SyncWeWork = newSyncWeWorkUseCase(db, uc.WeWork, uc.Employee, uc.Department, uc.Auth, uc.Tag)

	uc.AutoMigrate(context.Background())
	uc.AutoInit()

	return uc, func() {
		_ = sqlDB.Close()
	}
}

func (p *PowerXUseCase) AutoMigrate(ctx context.Context) {
	p.db.AutoMigrate(&CasbinPolicy{}, &AuthRole{}, &AuthRestAction{}, &AuthRecourse{})
	p.db.AutoMigrate(&Department{}, &Employee{}, &LiveQRCode{})
	p.db.AutoMigrate(&WeWorkDepartment{}, &WeWorkEmployee{})
}

func (p *PowerXUseCase) AutoInit() {
	p.Auth.Init()
	p.Department.Init()
}
