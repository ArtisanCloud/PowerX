package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchArtisanLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchArtisanLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchArtisanLogic {
	return &PatchArtisanLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchArtisanLogic) PatchArtisan(req *types.PatchArtisanRequest) (resp *types.PatchArtisanReply, err error) {
	// todo: add your logic here and delete this line

	return
}
