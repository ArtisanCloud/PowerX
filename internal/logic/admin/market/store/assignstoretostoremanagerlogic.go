package store

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignStoreToStoreManagerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignStoreToStoreManagerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignStoreToStoreManagerLogic {
	return &AssignStoreToStoreManagerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignStoreToStoreManagerLogic) AssignStoreToStoreManager(req *types.AssignStoreManagerRequest) (resp *types.AssignStoreManagerReply, err error) {
	// todo: add your logic here and delete this line

	return
}
