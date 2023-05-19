package artisan

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutArtisanLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutArtisanLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutArtisanLogic {
	return &PutArtisanLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutArtisanLogic) PutArtisan(req *types.PutArtisanRequest) (resp *types.PutArtisanReply, err error) {
	// todo: add your logic here and delete this line

	return
}
