package category

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCategoryLogic {
	return &GetCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetCategoryLogic) GetCategory(req *types.GetCategoryRequest) (resp *types.GetCategoryReply, err error) {

	productCategory, err := l.svcCtx.PowerX.Category.GetCategory(l.ctx, req.CategoryId)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetCategoryReply{
		Category: TransformCategoryToReply(productCategory),
	}, nil
}
