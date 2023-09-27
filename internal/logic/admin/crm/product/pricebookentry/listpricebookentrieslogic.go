package pricebookentry

import (
	"PowerX/internal/model/product"
	product2 "PowerX/internal/uc/powerx/crm/product"
	"context"
	"math"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListPriceBookEntriesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListPriceBookEntriesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPriceBookEntriesLogic {
	return &ListPriceBookEntriesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListPriceBookEntriesLogic) ListPriceBookEntries(req *types.ListPriceBookEntriesPageRequest) (resp *types.ListPriceBookEntriesPageReply, err error) {

	opt := &product2.FindPriceBookEntryOption{
		PriceBookId: req.PriceBookId,
		DontNeedSku: true,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	}

	priceBookPageEntryList := l.svcCtx.PowerX.PriceBookEntry.FindManyPriceBookEntries(l.ctx, opt)
	resp = &types.ListPriceBookEntriesPageReply{}

	resp.List = TransformAllPriceBookEntriesToAllPriceBookEntriesReply(priceBookPageEntryList.List)
	resp.PageIndex = priceBookPageEntryList.PageIndex
	resp.PageSize = priceBookPageEntryList.PageSize
	resp.Total = priceBookPageEntryList.Total

	return resp, nil
}

func TransformPriceEntriesToActivePriceEntryReply(entries []*product.PriceBookEntry) (entryReply *types.ActivePriceEntry) {
	for _, entry := range entries {
		if entry.SkuId == 0 && entry.IsActive {
			discount := CalDiscount(entry.UnitPrice, entry.ListPrice)
			return &types.ActivePriceEntry{
				UnitPrice: entry.UnitPrice,
				ListPrice: entry.ListPrice,
				Discount:  discount,
			}
		}
	}
	return nil
}

func TransformPriceBookEntriesToPriceBookEntriesReply(entries []*product.PriceBookEntry) (entriesReply []*types.PriceBookEntry) {
	//fmt.Dump(entries)
	entriesReply = []*types.PriceBookEntry{}
	for _, entry := range entries {
		if entry.SkuId == 0 {
			entriesReply = append(entriesReply, TransformPriceBookEntryToPriceBookEntryReply(entry))
		}
	}

	return entriesReply
}

func TransformAllPriceBookEntriesToAllPriceBookEntriesReply(entries []*product.PriceBookEntry) (entriesReply []*types.PriceBookEntry) {
	//fmt.Dump(entries)
	entriesReply = []*types.PriceBookEntry{}
	for _, entry := range entries {
		entriesReply = append(entriesReply, TransformPriceBookEntryToPriceBookEntryReply(entry))
	}

	return entriesReply
}

func CalDiscount(unitPrice float64, listPrice float64) float32 {
	discount := (unitPrice / listPrice) * 100
	discount = math.Round(discount*10) / 10 // 四舍五入保留一位小数
	return float32(discount)
}
