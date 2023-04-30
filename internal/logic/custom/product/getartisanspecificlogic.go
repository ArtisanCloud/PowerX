package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetArtisanSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetArtisanSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetArtisanSpecificLogic {
	return &GetArtisanSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetArtisanSpecificLogic) GetArtisanSpecific(req *types.GetArtisanSpecificRequest) (resp *types.GetArtisanSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}
