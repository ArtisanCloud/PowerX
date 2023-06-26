package dictionary

import (
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListDictionaryPageTypesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListDictionaryPageTypesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListDictionaryPageTypesLogic {
	return &ListDictionaryPageTypesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListDictionaryPageTypesLogic) ListDictionaryPageTypes(req *types.ListDictionaryTypesPageRequest) (resp *types.ListDictionaryTypesPageReply, err error) {
	page, err := l.svcCtx.PowerX.DataDictionary.FindManyDataDictionaryType(l.ctx, &powerx.FindManyDataDictTypeOption{
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := make([]types.DictionaryType, 0, len(page.List))
	for _, itemType := range page.List {

		var items = []*types.DictionaryItem{}
		if len(itemType.Items) > 0 {
			items = TransformItemsToItemsReply(itemType.Items)
		}

		list = append(list, types.DictionaryType{
			Id:          itemType.Id,
			Type:        itemType.Type,
			Name:        itemType.Name,
			Description: itemType.Description,
			Items:       items,
		})
	}

	return &types.ListDictionaryTypesPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil
}
