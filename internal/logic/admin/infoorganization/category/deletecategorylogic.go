package category

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteCategoryLogic {
	return &DeleteCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteCategoryLogic) DeleteCategory(req *types.DeleteCategoryRequest) (resp *types.DeleteCategoryReply, err error) {

	productCategory, err := l.svcCtx.PowerX.Category.GetCategory(l.ctx, req.Id)
	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}
	if len(productCategory.Children) > 0 {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "该品类包含子子品类，请处理子品类")
	}

	err = l.svcCtx.PowerX.Category.DeleteCategory(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &types.DeleteCategoryReply{
		Id: req.Id,
	}, nil
}
