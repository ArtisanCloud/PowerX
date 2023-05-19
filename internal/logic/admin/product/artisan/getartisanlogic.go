package artisan

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetArtisanLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetArtisanLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetArtisanLogic {
	return &GetArtisanLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetArtisanLogic) GetArtisan(req *types.GetArtisanRequest) (resp *types.GetArtisanReply, err error) {
	// todo: add your logic here and delete this line

	return
}
