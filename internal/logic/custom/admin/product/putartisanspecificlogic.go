package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutArtisanSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutArtisanSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutArtisanSpecificLogic {
	return &PutArtisanSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutArtisanSpecificLogic) PutArtisanSpecific(req *types.PutArtisanSpecificRequest) (resp *types.PutArtisanSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}
