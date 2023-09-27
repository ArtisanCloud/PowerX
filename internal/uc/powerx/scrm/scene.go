package scrm

import (
	"PowerX/internal/uc/powerx/scrm/scene"
	"github.com/robfig/cron/v3"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"gorm.io/gorm"
)

type SceneUseCase struct {
	db    *gorm.DB
	kv    *redis.Redis
	Cron  *cron.Cron
	Scene scene.IsceneInterface
}

func NewSceneUseCase(db *gorm.DB, kv *redis.Redis) *SceneUseCase {
	return &SceneUseCase{
		db:    db,
		Scene: scene.Repo(db, kv),
	}
}
