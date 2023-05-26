package category

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchProductCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchProductCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchProductCategoryLogic {
	return &PatchProductCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchProductCategoryLogic) PatchProductCategory(req *types.PatchProductCategoryRequest) (resp *types.PatchProductCategoryReply, err error) {

	productCategory := &product.ProductCategory{
		PowerModel: powermodel.PowerModel{
			Id: req.Id,
		},
		PId: req.PId,
	}

	l.svcCtx.PowerX.ProductCategory.PatchProductCategory(l.ctx, req.Id, productCategory)

	return &types.PatchProductCategoryReply{
		ProductCategory: types.ProductCategory{
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
