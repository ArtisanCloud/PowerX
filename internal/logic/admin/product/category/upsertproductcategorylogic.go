package category

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpsertProductCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpsertProductCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpsertProductCategoryLogic {
	return &UpsertProductCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpsertProductCategoryLogic) UpsertProductCategory(req *types.UpsertProductCategoryRequest) (resp *types.UpsertProductCategoryReply, err error) {

	productCategory := &product.ProductCategory{
		PowerModel: powermodel.PowerModel{
			Id: req.Id,
		},
		PId:         req.PId,
		Name:        req.Name,
		Sort:        req.Sort,
		ViceName:    req.ViceName,
		Description: req.Description,
		ImageAbleInfo: model.ImageAbleInfo{
			Icon:            req.Icon,
			BackgroundColor: req.BackgroundColor,
			ImageURL:        req.ImageURL,
		},
	}

	productCategory, err = l.svcCtx.PowerX.ProductCategory.UpsertProductCategory(l.ctx, productCategory)

	if err != nil {
		panic(err)
		return
	}

	return &types.UpsertProductCategoryReply{
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
