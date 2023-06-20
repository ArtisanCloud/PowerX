package pricebookentry

import (
	"PowerX/internal/model/product"
	fmt "PowerX/pkg/printx"
	"context"
	"github.com/kr/pretty"
	"github.com/pkg/errors"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ConfigPriceBookEntryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewConfigPriceBookEntryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfigPriceBookEntryLogic {
	return &ConfigPriceBookEntryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ConfigPriceBookEntryLogic) ConfigPriceBookEntry(req *types.ConfigPriceBookEntryRequest) (resp *types.ConfigPriceBookEntryReply, err error) {

	entries, err := flattenPriceBookEntries(req)
	if err != nil {
		return nil, err
	}
	fmt.Dump(entries)

	entries, err = l.svcCtx.PowerX.PriceBookEntry.UpsertPriceBookEntries(l.ctx, entries)
	if err != nil {
		return nil, err
	}

	return &types.ConfigPriceBookEntryReply{
		PriceBookEntries: TransformPriceBookEntriesToPriceBookEntriesReply(entries),
	}, err
}

func TransformPriceBookEntryRequestToPriceBook(entryRequest *types.PriceBookEntry) *product.PriceBookEntry {

	if entryRequest == nil {
		return nil
	}

	entry := &product.PriceBookEntry{
		PriceBookId: entryRequest.PriceBookId,
		ProductId:   entryRequest.ProductId,
		SkuId:       entryRequest.SkuId,
		UnitPrice:   entryRequest.UnitPrice,
		ListPrice:   entryRequest.ListPrice,
		IsActive:    entryRequest.IsActive,
	}
	entry.UniqueID = entry.GetComposedUniqueID()

	return entry
}

func flattenPriceBookEntries(req *types.ConfigPriceBookEntryRequest) (entries []*product.PriceBookEntry, err error) {

	entries = []*product.PriceBookEntry{}
	for i, entry := range req.PriceBookEntries {
		if entry.PriceBookId <= 0 || entry.ProductId <= 0 {
			return nil, errors.New(pretty.Sprintf("price book entry index: %d is not valid", i))
		}
		entries = append(entries, TransformPriceBookEntryRequestToPriceBook(&entry))
		if len(entry.SKUEntries) > 0 {
			for j, skuEntry := range entry.SKUEntries {
				if skuEntry.PriceBookId <= 0 || skuEntry.ProductId <= 0 || skuEntry.SkuId <= 0 {
					return nil, errors.New(pretty.Sprintf("price book sku entry index: [%d %d] is not valid", i, j))
				}
				entries = append(entries, TransformPriceBookEntryRequestToPriceBook(skuEntry))
			}
		}
	}

	return entries, nil

}
