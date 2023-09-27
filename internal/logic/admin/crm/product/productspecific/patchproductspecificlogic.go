package productspecific

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchProductSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchProductSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchProductSpecificLogic {
	return &PatchProductSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchProductSpecificLogic) PatchProductSpecific(req *types.PatchProductSpecificRequest) (resp *types.PatchProductSpecificReply, err error) {
	return
}
