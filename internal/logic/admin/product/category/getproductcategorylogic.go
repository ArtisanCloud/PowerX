package category

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetProductCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetProductCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetProductCategoryLogic {
	return &GetProductCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetProductCategoryLogic) GetProductCategory(req *types.GetProductCategoryRequest) (resp *types.GetProductCategoryReply, err error) {
	// todo: add your logic here and delete this line

	return
}
