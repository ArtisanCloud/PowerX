package scene

import (
	"PowerX/internal/model/scene"
	"context"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"gorm.io/gorm"
)

var Scene IsceneInterface = new(sceneUseCase)

type sceneUseCase struct {
	db  *gorm.DB
	kv  *redis.Redis
	ctx context.Context
	modelSceneQRCode
}
type (
	modelSceneQRCode struct {
		qrcode scene.SceneQRCode
	}
)

// NewOrganizationUseCase
//
//	@Description:
//	@param db
//	@param wework
//	@return iUserInterface
func Repo(db *gorm.DB, kv *redis.Redis) IsceneInterface {

	return &sceneUseCase{
		db:  db,
		kv:  kv,
		ctx: context.TODO(),
	}

}
