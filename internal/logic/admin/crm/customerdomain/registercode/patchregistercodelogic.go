package registercode

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchRegisterCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchRegisterCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchRegisterCodeLogic {
	return &PatchRegisterCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchRegisterCodeLogic) PatchRegisterCode(req *types.PatchRegisterCodeRequest) (resp *types.PatchRegisterCodeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
