package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchServiceSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchServiceSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchServiceSpecificLogic {
	return &PatchServiceSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchServiceSpecificLogic) PatchServiceSpecific(req *types.PatchServiceSpecificRequest) (resp *types.PatchServiceSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}
