package artisan

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateArtisanLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateArtisanLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateArtisanLogic {
	return &CreateArtisanLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateArtisanLogic) CreateArtisan(req *types.CreateArtisanRequest) (resp *types.CreateArtisanReply, err error) {
	// todo: add your logic here and delete this line

	return
}
