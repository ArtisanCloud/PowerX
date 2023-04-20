package dictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetDictionaryTypeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetDictionaryTypeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDictionaryTypeLogic {
	return &GetDictionaryTypeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetDictionaryTypeLogic) GetDictionaryType(req *types.GetDictionaryTypeRequest) (resp *types.GetDictionaryTypeReply, err error) {

	itemType, err := l.svcCtx.PowerX.DataDictionary.GetDataDictionaryType(l.ctx, req.DictionaryType)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	var items = []*types.DictionaryItem{}
	if len(itemType.Items) > 0 {
		items = TransformItemsToItemsReply(itemType.Items)
	}

	return &types.GetDictionaryTypeReply{
		&types.DictionaryType{
			Type:        itemType.Type,
			Name:        itemType.Name,
			Description: itemType.Description,
			Items:       items,
		},
	}, nil
}

func TransformItemsToItemsReply(Items []*model.DataDictionaryItem) (itemsReply []*types.DictionaryItem) {

	itemsReply = []*types.DictionaryItem{}
	for _, item := range Items {
		itemsReply = append(itemsReply, TransformItemToItemReply(item))
	}
	return itemsReply
}
