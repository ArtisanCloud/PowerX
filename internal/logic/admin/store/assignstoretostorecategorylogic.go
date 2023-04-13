package store

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignStoreToStoreCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignStoreToStoreCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignStoreToStoreCategoryLogic {
	return &AssignStoreToStoreCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignStoreToStoreCategoryLogic) AssignStoreToStoreCategory(req *types.AssignStoreManagerRequest) (resp *types.AssignStoreManagerReply, err error) {
	// todo: add your logic here and delete this line

	return
}
