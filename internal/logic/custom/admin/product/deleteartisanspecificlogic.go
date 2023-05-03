package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteArtisanSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteArtisanSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteArtisanSpecificLogic {
	return &DeleteArtisanSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteArtisanSpecificLogic) DeleteArtisanSpecific(req *types.DeleteArtisanSpecificRequest) (resp *types.DeleteArtisanSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}
