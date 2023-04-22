package product

import (
	"PowerX/internal/model"
	"PowerX/internal/model/product"
	"PowerX/internal/uc/powerx"
	"context"
	"gorm.io/datatypes"
	"time"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateProductLogic {
	return &CreateProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateProductLogic) CreateProduct(req *types.CreateProductRequest) (resp *types.CreateProductReply, err error) {

	mdlProduct := TransformProductRequestToProduct(&(req.Product))

	if len(req.SalesChannelsItemIds) > 0 {
		salesChannelsItems, err := l.svcCtx.PowerX.DataDictionary.FindAllDictionaryItems(l.ctx, &powerx.FindManyDataDictItemOption{
			Ids: req.SalesChannelsItemIds,
		})
		if err != nil {
			return nil, err
		}
		mdlProduct.PivotSalesChannels, err = (&model.PivotDataDictionaryToObject{}).MakeMorphPivotsFromObjectToDDs(mdlProduct, salesChannelsItems)
	}

	if len(req.PromoteChannelsItemIds) > 0 {
		promoteChannelsItems, err := l.svcCtx.PowerX.DataDictionary.FindAllDictionaryItems(l.ctx, &powerx.FindManyDataDictItemOption{
			Ids: req.PromoteChannelsItemIds,
		})
		if err != nil {
			return nil, err
		}
		mdlProduct.PivotPromoteChannels, err = (&model.PivotDataDictionaryToObject{}).MakeMorphPivotsFromObjectToDDs(mdlProduct, promoteChannelsItems)
	}

	l.svcCtx.PowerX.Product.CreateProduct(l.ctx, mdlProduct)

	return &types.CreateProductReply{
		mdlProduct.Id,
	}, nil
}

func TransformProductRequestToProduct(productRequest *types.Product) (mdlProduct *product.Product) {

	saleStartDate, _ := time.Parse("format", productRequest.SaleStartDate)
	saleEndDate, _ := time.Parse("format", productRequest.SaleEndDate)

	return &product.Product{
		Name:               productRequest.Name,
		Type:               productRequest.Type,
		Plan:               productRequest.Plan,
		AccountingCategory: productRequest.AccountingCategory,
		CanSellOnline:      productRequest.CanSellOnline,
		CanUseForDeduct:    productRequest.CanUseForDeduct,
		ApprovalStatus:     productRequest.ApprovalStatus,
		IsActivated:        productRequest.IsActivated,
		Description:        productRequest.Description,
		CoverURL:           productRequest.CoverURL,
		PurchasedQuantity:  productRequest.PurchasedQuantity,
		ValidityPeriodDays: productRequest.ValidityPeriodDays,
		SaleStartDate:      saleStartDate,
		SaleEndDate:        saleEndDate,
		ProductSpecific: product.ProductSpecific{
			Inventory: productRequest.Inventory,
			Weight:    productRequest.Weight,
			Volume:    productRequest.Volume,
			Encode:    productRequest.Encode,
			BarCode:   productRequest.BarCode,
			Extra:     datatypes.JSON(productRequest.Extra),
		},
	}

}

func TransformProductToProductReply(mdlProduct *product.Product) (productReply *types.Product) {

	getItemIds := func(items []*model.DataDictionaryItem) []int64 {
		arrayIds := []int64{}
		for _, item := range items {
			arrayIds = append(arrayIds, item.Id)
		}
		return arrayIds
	}

	return &types.Product{
		Id:                 mdlProduct.Id,
		Name:               mdlProduct.Name,
		Type:               mdlProduct.Type,
		Plan:               mdlProduct.Plan,
		AccountingCategory: mdlProduct.AccountingCategory,
		CanSellOnline:      mdlProduct.CanSellOnline,
		CanUseForDeduct:    mdlProduct.CanUseForDeduct,
		ApprovalStatus:     mdlProduct.ApprovalStatus,
		IsActivated:        mdlProduct.IsActivated,
		Description:        mdlProduct.Description,
		CoverURL:           mdlProduct.CoverURL,
		PurchasedQuantity:  mdlProduct.PurchasedQuantity,
		ValidityPeriodDays: mdlProduct.ValidityPeriodDays,
		SaleStartDate:      mdlProduct.SaleStartDate.String(),
		SaleEndDate:        mdlProduct.SaleEndDate.String(),
		ProductSpecific: types.ProductSpecific{
			Inventory: mdlProduct.Inventory,
			Weight:    mdlProduct.Weight,
			Volume:    mdlProduct.Volume,
			Encode:    mdlProduct.Encode,
			BarCode:   mdlProduct.BarCode,
			Extra:     mdlProduct.Extra.String(),
		},
		SalesChannelsItemIds:   getItemIds(mdlProduct.SalesChannels),
		PromoteChannelsItemIds: getItemIds(mdlProduct.PromoteChannels),
	}

}
