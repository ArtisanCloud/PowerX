package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignProductToProductCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignProductToProductCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignProductToProductCategoryLogic {
	return &AssignProductToProductCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignProductToProductCategoryLogic) AssignProductToProductCategory(req *types.AssignProductToProductCategoryRequest) (resp *types.AssignProductToProductCategoryReply, err error) {
	// todo: add your logic here and delete this line

	return
}
