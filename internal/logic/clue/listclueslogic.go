package clue

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListCluesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListCluesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListCluesLogic {
	return &ListCluesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListCluesLogic) ListClues(req *types.ListCluesRequest) (resp *types.ListCluesReply, err error) {
	// todo: add your logic here and delete this line

	return
}
