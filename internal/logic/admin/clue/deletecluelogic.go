package clue

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteClueLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteClueLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteClueLogic {
	return &DeleteClueLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteClueLogic) DeleteClue() (resp *types.DeleteClueReply, err error) {
	// todo: add your logic here and delete this line

	return
}
