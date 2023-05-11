package dictionary

import (
	"PowerX/internal/logic/admin/dictionary"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetDictionaryItemLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetDictionaryItemLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDictionaryItemLogic {
	return &GetDictionaryItemLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetDictionaryItemLogic) GetDictionaryItem(req *types.GetDictionaryItemRequest) (resp *types.GetDictionaryItemReply, err error) {
	item, err := l.svcCtx.PowerX.DataDictionary.GetDataDictionaryItem(l.ctx, req.DictionaryType, req.DictionaryItem)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetDictionaryItemReply{
		DictionaryItem: dictionary.TransformItemToItemReply(item),
	}, nil
}
