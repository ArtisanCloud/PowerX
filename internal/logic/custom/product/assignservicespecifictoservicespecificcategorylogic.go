package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignServiceSpecificToServiceSpecificCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignServiceSpecificToServiceSpecificCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignServiceSpecificToServiceSpecificCategoryLogic {
	return &AssignServiceSpecificToServiceSpecificCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignServiceSpecificToServiceSpecificCategoryLogic) AssignServiceSpecificToServiceSpecificCategory(req *types.AssignServiceSpecificManagerRequest) (resp *types.AssignServiceSpecificManagerReply, err error) {
	// todo: add your logic here and delete this line

	return
}
