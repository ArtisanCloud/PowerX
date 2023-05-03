package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchArtisanSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchArtisanSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchArtisanSpecificLogic {
	return &PatchArtisanSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchArtisanSpecificLogic) PatchArtisanSpecific(req *types.PatchArtisanSpecificRequest) (resp *types.PatchArtisanSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}
