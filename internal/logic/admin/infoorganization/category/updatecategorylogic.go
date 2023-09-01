package category

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"context"

	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateCategoryLogic {
	return &UpdateCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateCategoryLogic) UpdateCategory(req *types.UpdateCategoryRequest) (resp *types.UpdateCategoryReply, err error) {
	category := TransformRequestToCategory(&req.Category)
	category.PowerModel = powermodel.PowerModel{
		Id: req.Id,
	}

	category, err = l.svcCtx.PowerX.Category.UpsertCategory(l.ctx, category)

	if err != nil {

	}

	return &types.UpdateCategoryReply{
		Id: category.Id,
	}, nil
}
