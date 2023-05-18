package product

import (
	"PowerX/internal/model"
	"PowerX/internal/model/media"
	"PowerX/internal/model/product"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"math"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetProductLogic {
	return &GetProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetProductLogic) GetProduct(req *types.GetProductRequest) (resp *types.GetProductReply, err error) {
	mdlProduct, err := l.svcCtx.PowerX.Product.GetProduct(l.ctx, req.ProductId)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetProductReply{
		Product: TransformProductToProductReplyToMP(mdlProduct),
	}, nil
}

func TransformProductToProductReplyToMP(mdlProduct *product.Product) (productReply *types.Product) {

	getItemIds := func(items []*model.PivotDataDictionaryToObject) []int64 {
		arrayIds := []int64{}
		if len(items) <= 0 {
			return arrayIds
		}
		for _, item := range items {
			if item.DataDictionaryItem != nil {
				arrayIds = append(arrayIds, item.DataDictionaryItem.Id)
			}
		}
		return arrayIds
	}

	getCategoryIds := func(categories []*product.ProductCategory) []int64 {
		arrayIds := []int64{}
		if len(categories) <= 0 {
			return arrayIds
		}
		for _, category := range categories {
			arrayIds = append(arrayIds, category.Id)
		}
		return arrayIds
	}

	//getImageIds := func(pivots []*media.PivotMediaResourceToObject) []int64 {
	//	arrayIds := []int64{}
	//	if len(pivots) <= 0 {
	//		return arrayIds
	//	}
	//	for _, pivot := range pivots {
	//		arrayIds = append(arrayIds, pivot.MediaResourceId)
	//	}
	//	return arrayIds
	//}

	return &types.Product{
		Id:                     mdlProduct.Id,
		Name:                   mdlProduct.Name,
		Description:            mdlProduct.Description,
		ProductCategories:      TransformProductCategoriesToProductCategoriesReplyToMP(mdlProduct.ProductCategories),
		SalesChannelsItemIds:   getItemIds(mdlProduct.PivotSalesChannels),
		PromoteChannelsItemIds: getItemIds(mdlProduct.PivotPromoteChannels),
		CategoryIds:            getCategoryIds(mdlProduct.ProductCategories),
		ProductSpecifics:       TransformSpecificsToSpecificsReplyToMP(mdlProduct.ProductSpecifics),
		PriceEntry:             TransformPriceEntryToPriceEntryReplyToMP(mdlProduct.PriceBookEntries),
		SKUs:                   TransformSkusToSkusReplyToMP(mdlProduct.SKUs),
		ProductAttribute: &types.ProductAttribute{
			Inventory:  mdlProduct.Inventory,
			SoldAmount: mdlProduct.SoldAmount,
		},
		//CoverImageIds:          getImageIds(mdlProduct.PivotCoverImages),
		//DetailImageIds:         getImageIds(mdlProduct.PivotDetailImages),
		CoverImages:  TransformProductImagesToImagesReplyToMP(mdlProduct.PivotCoverImages),
		DetailImages: TransformProductImagesToImagesReplyToMP(mdlProduct.PivotDetailImages),
	}

}

func TransformPriceEntryToPriceEntryReplyToMP(entries []*product.PriceBookEntry) (entriesReply *types.PriceEntry) {
	//fmt.Dump(entries)
	for _, entry := range entries {
		if entry.SkuId == 0 && entry.IsActive {
			discount := (entry.UnitPrice / entry.RetailPrice) * 100
			discount = math.Round(discount*10) / 10 // 四舍五入保留一位小数

			return &types.PriceEntry{
				Id:          entry.Id,
				UnitPrice:   entry.UnitPrice,
				RetailPrice: entry.RetailPrice,
				Discount:    discount,
			}
		}
	}

	return nil
}

func TransformProductImagesToImagesReplyToMP(pivots []*media.PivotMediaResourceToObject) (imagesReply []*types.ProductImage) {

	imagesReply = []*types.ProductImage{}
	for _, pivot := range pivots {
		imageReply := TransformProductImageToImageReplyToMP(pivot.MediaResource)
		imagesReply = append(imagesReply, imageReply)
	}
	return imagesReply
}

func TransformProductImageToImageReplyToMP(resource *media.MediaResource) (imagesReply *types.ProductImage) {
	if resource == nil {
		return nil
	}
	return &types.ProductImage{
		Id:   resource.Id,
		Url:  resource.Url,
		Name: resource.Filename,
	}
}

func TransformSkusToSkusReplyToMP(skus []*product.SKU) (skusReply []*types.SKU) {

	skusReply = []*types.SKU{}
	for _, sku := range skus {
		skuReply := TransformSkuToSkuReplyToMP(sku)
		skusReply = append(skusReply, skuReply)
	}
	return skusReply
}

func TransformSkuToSkuReplyToMP(sku *product.SKU) (skuReply *types.SKU) {
	if sku == nil {
		return nil
	}

	if !sku.PriceBookEntry.IsActive {
		return nil
	}

	unitPrice := 0.0
	retailPrice := 0.0
	if sku.PriceBookEntry != nil {
		unitPrice = sku.PriceBookEntry.UnitPrice
		retailPrice = sku.PriceBookEntry.RetailPrice
	}
	var optionsIds = []int64{}
	for _, pivot := range sku.PivotSkuToSpecificOptions {
		if pivot.IsActivated {
			optionsIds = append(optionsIds, pivot.SpecificOptionId)
		}
	}

	return &types.SKU{
		Id:          sku.Id,
		SkuNo:       sku.SkuNo,
		Inventory:   sku.Inventory,
		UnitPrice:   unitPrice,
		RetailPrice: retailPrice,
		OptionsIds:  optionsIds,
	}
}

func TransformSpecificsToSpecificsReplyToMP(specifics []*product.ProductSpecific) (specificReplies []*types.ProductSpecific) {

	specificReplies = []*types.ProductSpecific{}
	for _, specific := range specifics {
		specificReply := TransformSpecificToSpecificReplyToMP(specific)
		specificReplies = append(specificReplies, specificReply)
	}
	return specificReplies
}

func TransformSpecificToSpecificReplyToMP(specific *product.ProductSpecific) (imagesReply *types.ProductSpecific) {
	if specific == nil {
		return nil
	}
	return &types.ProductSpecific{
		Id:              specific.Id,
		Name:            specific.Name,
		SpecificOptions: TransformSpecificOptionsToSpecificOptionsReplyToMP(specific.Options),
	}
}

func TransformSpecificOptionsToSpecificOptionsReplyToMP(options []*product.SpecificOption) (optionsReply []*types.SpecificOption) {

	optionsReply = []*types.SpecificOption{}
	for _, option := range options {
		specificReply := TransformSpecificOptionToSpecificOptionReplyToMP(option)
		if specificReply != nil {
			optionsReply = append(optionsReply, specificReply)
		}
	}
	return optionsReply
}

func TransformSpecificOptionToSpecificOptionReplyToMP(option *product.SpecificOption) (imagesReply *types.SpecificOption) {
	if option == nil {
		return nil
	}
	if !option.IsActivated {
		return nil
	}
	return &types.SpecificOption{
		Id:          option.Id,
		Name:        option.Name,
		IsActivated: option.IsActivated,
	}
}
