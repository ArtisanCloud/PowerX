package clue

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateCluesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateCluesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateCluesLogic {
	return &CreateCluesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateCluesLogic) CreateClues(req *types.CreateCluesRequest) (resp *types.CreateCluesReply, err error) {
	// todo: add your logic here and delete this line

	return
}
