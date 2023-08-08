package category

import (
	"PowerX/internal/model"
	"PowerX/internal/model/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateProductCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateProductCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateProductCategoryLogic {
	return &CreateProductCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateProductCategoryLogic) CreateProductCategory(req *types.CreateProductCategoryRequest) (resp *types.CreateProductCategoryReply, err error) {
	productCategory := TransformRequestToProductCategory(&req.ProductCategory)

	productCategory, err = l.svcCtx.PowerX.ProductCategory.UpsertProductCategory(l.ctx, productCategory)
	if err != nil {
		return nil, err
	}

	return &types.CreateProductCategoryReply{
		ProductCategory: TransformProductCategoryToReply(productCategory),
	}, nil
}

func TransformRequestToProductCategory(req *types.ProductCategory) *product.ProductCategory {
	return &product.ProductCategory{
		PId:          req.PId,
		Name:         req.Name,
		Sort:         req.Sort,
		ViceName:     req.ViceName,
		Description:  req.Description,
		CoverImageId: req.CoverImageId,
		ImageAbleInfo: model.ImageAbleInfo{
			Icon:            req.Icon,
			BackgroundColor: req.BackgroundColor,
		},
	}
}
