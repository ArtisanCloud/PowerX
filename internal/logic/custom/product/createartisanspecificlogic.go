package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateArtisanSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateArtisanSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateArtisanSpecificLogic {
	return &CreateArtisanSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateArtisanSpecificLogic) CreateArtisanSpecific(req *types.CreateArtisanSpecificRequest) (resp *types.CreateArtisanSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}
