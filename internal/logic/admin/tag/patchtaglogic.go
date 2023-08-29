package tag

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchTagLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchTagLogic {
	return &PatchTagLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchTagLogic) PatchTag(req *types.PatchTagRequest) (resp *types.PatchTagReply, err error) {
	// todo: add your logic here and delete this line

	return
}
