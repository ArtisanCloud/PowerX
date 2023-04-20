package dictionary

import (
	"PowerX/internal/model"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateDictionaryItemLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateDictionaryItemLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateDictionaryItemLogic {
	return &UpdateDictionaryItemLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateDictionaryItemLogic) UpdateDictionaryItem(req *types.UpdateDictionaryItemRequest) (resp *types.UpdateDictionaryItemReply, err error) {
	newModel := model.DataDictionaryItem{
		Name:        req.Name,
		Value:       req.Value,
		Sort:        req.Sort,
		Description: req.Description,
	}
	if err := l.svcCtx.PowerX.DataDictionary.PatchDataDictionaryItem(l.ctx, req.Type, req.Key, &newModel); err != nil {
		return nil, err
	}

	return &types.UpdateDictionaryItemReply{
		DictionaryItem: &types.DictionaryItem{
			Key:         req.Key,
			Type:        req.Type,
			Name:        newModel.Name,
			Value:       newModel.Value,
			Description: newModel.Description,
		},
	}, nil
}
