package product

import (
	"PowerX/internal/types/errorx"
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

	productCategory, err := l.svcCtx.PowerX.ProductCategory.GetProductCategory(l.ctx, req.ProductCategoryId)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetProductCategoryReply{
		ProductCategory: &types.ProductCategory{
			Id:          productCategory.Id,
			PId:         productCategory.PId,
			Name:        productCategory.Name,
			Sort:        productCategory.Sort,
			ViceName:    productCategory.ViceName,
			Description: productCategory.Description,
			CreatedAt:   productCategory.CreatedAt.String(),
			ImageAbleInfo: types.ImageAbleInfo{
				Icon:            productCategory.Icon,
				BackgroundColor: productCategory.BackgroundColor,
				ImageURL:        productCategory.ImageURL,
			},
		},
	}, nil
}
