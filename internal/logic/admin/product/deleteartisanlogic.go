package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteArtisanLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteArtisanLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteArtisanLogic {
	return &DeleteArtisanLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteArtisanLogic) DeleteArtisan(req *types.DeleteArtisanRequest) (resp *types.DeleteArtisanReply, err error) {
	// todo: add your logic here and delete this line

	return
}
