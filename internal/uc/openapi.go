package uc

import (
	"PowerX/internal/config"
	"PowerX/internal/uc/openapi"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type OpenAPIUseCase struct {
	db    *gorm.DB
	redis *redis.Redis

	Auth openapi.AuthorizationOpenAPIPlatformUseCase
}

func NewOpenAPIUseCase(conf *config.Config, pxUseCase *PowerXUseCase) (uc *OpenAPIUseCase, clean func()) {

	db, err := gorm.Open(postgres.Open(conf.PowerXDatabase.DSN), &gorm.Config{
		//Logger:                                   logger.Default.LogMode(logger.Info),
		DisableForeignKeyConstraintWhenMigrating: true,
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

	uc = &OpenAPIUseCase{
		db: db,
		redis: redis.New(conf.RedisBase.Host, func(r *redis.Redis) {
			r.Pass = conf.RedisBase.Password
		}),
	}

	return uc, func() {
		_ = sqlDB.Close()
	}
}
