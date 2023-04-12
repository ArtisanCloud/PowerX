package dictionary

import (
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetDictionaryItemsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetDictionaryItemsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDictionaryItemsLogic {
	return &GetDictionaryItemsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetDictionaryItemsLogic) GetDictionaryItems(req *types.GetDictionaryItemsRequest) (resp *types.GetDictionaryItemsReply, err error) {
	page, err := l.svcCtx.PowerX.DataDictionaryUserCase.FindManyDataDictionaryItem(l.ctx, &powerx.FindManyDataDictItemOption{
		Types: req.Types,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})

	if err != nil {
		return nil, err
	}

	list := make([]types.DictionaryItem, 0, len(page.List))
	for _, item := range page.List {
		list = append(list, types.DictionaryItem{
			Key:         item.Key,
			Type:        item.Type,
			Name:        item.Name,
			Value:       item.Value,
			Description: item.Description,
		})
	}

	return &types.GetDictionaryItemsReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil
}
