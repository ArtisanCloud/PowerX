package clue

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchClueLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchClueLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchClueLogic {
	return &PatchClueLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchClueLogic) PatchClue(req *types.PatchClueRequest) (resp *types.PatchClueReply, err error) {
	// todo: add your logic here and delete this line

	return
}
