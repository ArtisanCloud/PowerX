package product

import (
	"PowerX/internal/logic/admin/crm/product/pricebookentry"
	"PowerX/internal/logic/mp/mediaresource"
	"PowerX/internal/model"
	"PowerX/internal/model/crm/product"
	"PowerX/internal/model/media"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
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
		Product: TransformProductToReplyForMP(mdlProduct),
	}, nil
}

func TransformProductToReplyForMP(mdlProduct *product.Product) (productReply *types.Product) {

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
		SPU:                    mdlProduct.SPU,
		Description:            mdlProduct.Description,
		ProductCategories:      TransformProductCategoriesToReplyForMP(mdlProduct.ProductCategories),
		SalesChannelsItemIds:   getItemIds(mdlProduct.PivotSalesChannels),
		PromoteChannelsItemIds: getItemIds(mdlProduct.PivotPromoteChannels),
		CategoryIds:            getCategoryIds(mdlProduct.ProductCategories),
		ProductSpecifics:       TransformSpecificsToReplyForMP(mdlProduct.ProductSpecifics),
		ActivePriceEntry:       TransformPriceEntryToReplyForMP(mdlProduct.PriceBookEntries),
		SKUs:                   TransformSkusToReplyForMP(mdlProduct.SKUs),
		ProductAttribute: &types.ProductAttribute{
			Inventory:  mdlProduct.Inventory,
			SoldAmount: mdlProduct.SoldAmount,
		},
		//CoverImageIds:          getImageIds(mdlProduct.PivotCoverImages),
		//DetailImageIds:         getImageIds(mdlProduct.PivotDetailImages),
		CoverImages:  mediaresource.TransformResourceMediasToReplyForMP(mdlProduct.PivotCoverImages),
		DetailImages: mediaresource.TransformResourceMediasToReplyForMP(mdlProduct.PivotDetailImages),
	}

}

func TransformPriceEntryToReplyForMP(entries []*product.PriceBookEntry) (entriesReply *types.ActivePriceEntry) {
	//fmt.Dump(entries)
	for _, entry := range entries {
		if entry.SkuId == 0 && entry.IsActive {
			discount := pricebookentry.CalDiscount(entry.UnitPrice, entry.ListPrice)
			return &types.ActivePriceEntry{
				Id:        entry.Id,
				UnitPrice: entry.UnitPrice,
				ListPrice: entry.ListPrice,
				Discount:  discount,
			}
		}
	}

	return nil
}

func TransformProductImageToReplyForMP(resource *media.MediaResource) (imagesReply *types.MediaResource) {
	if resource == nil {
		return nil
	}
	return &types.MediaResource{
		Id:            resource.Id,
		Url:           resource.Url,
		IsLocalStored: resource.IsLocalStored,
		Filename:      resource.Filename,
	}
}

func TransformSkusToReplyForMP(skus []*product.SKU) (skusReply []*types.SKU) {

	skusReply = []*types.SKU{}
	for _, sku := range skus {
		skuReply := TransformSkuToReplyForMP(sku)
		skusReply = append(skusReply, skuReply)
	}
	return skusReply
}

func TransformSkuToReplyForMP(sku *product.SKU) (skuReply *types.SKU) {
	if sku == nil {
		return nil
	}

	if !sku.PriceBookEntry.IsActive {
		return nil
	}

	unitPrice := 0.0
	listPrice := 0.0
	if sku.PriceBookEntry != nil {
		unitPrice = sku.PriceBookEntry.UnitPrice
		listPrice = sku.PriceBookEntry.ListPrice
	}
	var optionsIds = []int64{}
	for _, pivot := range sku.PivotSkuToSpecificOptions {
		if pivot.IsActivated {
			optionsIds = append(optionsIds, pivot.SpecificOptionId)
		}
	}

	return &types.SKU{
		Id:         sku.Id,
		SkuNo:      sku.SkuNo,
		Inventory:  sku.Inventory,
		UnitPrice:  unitPrice,
		ListPrice:  listPrice,
		OptionsIds: optionsIds,
	}
}

func TransformSpecificsToReplyForMP(specifics []*product.ProductSpecific) (specificReplies []*types.ProductSpecific) {

	specificReplies = []*types.ProductSpecific{}
	for _, specific := range specifics {
		specificReply := TransformSpecificToReplyForMP(specific)
		specificReplies = append(specificReplies, specificReply)
	}
	return specificReplies
}

func TransformSpecificToReplyForMP(specific *product.ProductSpecific) (imagesReply *types.ProductSpecific) {
	if specific == nil {
		return nil
	}
	return &types.ProductSpecific{
		Id:              specific.Id,
		Name:            specific.Name,
		SpecificOptions: TransformSpecificOptionsToReplyForMP(specific.Options),
	}
}

func TransformSpecificOptionsToReplyForMP(options []*product.SpecificOption) (optionsReply []*types.SpecificOption) {

	optionsReply = []*types.SpecificOption{}
	for _, option := range options {
		specificReply := TransformSpecificOptionToReplyForMP(option)
		if specificReply != nil {
			optionsReply = append(optionsReply, specificReply)
		}
	}
	return optionsReply
}

func TransformSpecificOptionToReplyForMP(option *product.SpecificOption) (imagesReply *types.SpecificOption) {
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
