package category

import (
	"PowerX/internal/model/powermodel"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateProductCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateProductCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateProductCategoryLogic {
	return &UpdateProductCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateProductCategoryLogic) UpdateProductCategory(req *types.UpdateProductCategoryRequest) (resp *types.UpdateProductCategoryReply, err error) {
	productCategory := TransformProductCategoryRequestToProductCategory(&req.ProductCategory)
	productCategory.PowerModel = powermodel.PowerModel{
		Id: req.Id,
	}

	productCategory, err = l.svcCtx.PowerX.ProductCategory.UpsertProductCategory(l.ctx, productCategory)

	if err != nil {

	}

	return &types.UpdateProductCategoryReply{
		Id: productCategory.Id,
	}, nil
}
