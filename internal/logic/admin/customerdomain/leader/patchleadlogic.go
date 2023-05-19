package leader

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchLeadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchLeadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchLeadLogic {
	return &PatchLeadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchLeadLogic) PatchLead(req *types.PatchLeadRequest) (resp *types.PatchLeadReply, err error) {
	// todo: add your logic here and delete this line

	return
}
