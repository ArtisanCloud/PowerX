package dictionary

import (
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListDictionaryItemsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListDictionaryItemsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListDictionaryItemsLogic {
	return &ListDictionaryItemsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListDictionaryItemsLogic) ListDictionaryItems(req *types.ListDictionaryItemsRequest) (resp *types.ListDictionaryItemsReply, err error) {
	dictionaryTypes, err := l.svcCtx.PowerX.DataDictionary.FindAllDictionaryItems(l.ctx, &powerx.FindManyDataDictItemOption{
		Types: []string{req.Type},
	})

	if err != nil {
		return nil, err
	}

	list := make([]types.DictionaryItem, 0, len(dictionaryTypes))
	for _, item := range dictionaryTypes {
		list = append(list, types.DictionaryItem{
			Key:         item.Key,
			Type:        item.Type,
			Name:        item.Name,
			Value:       item.Value,
			Description: item.Description,
		})
	}

	return &types.ListDictionaryItemsReply{
		List: list,
	}, nil

}
