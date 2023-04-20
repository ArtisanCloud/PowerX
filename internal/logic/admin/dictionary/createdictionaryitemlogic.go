package dictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateDictionaryItemLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateDictionaryItemLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateDictionaryItemLogic {
	return &CreateDictionaryItemLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateDictionaryItemLogic) CreateDictionaryItem(req *types.CreateDictionaryItemRequest) (resp *types.CreateDictionaryItemReply, err error) {
	if !l.svcCtx.PowerX.DataDictionary.TypeIsExist(l.ctx, req.Type) {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "类型不存在")
	}

	if l.svcCtx.PowerX.DataDictionary.ItemIsExist(l.ctx, req.Type, req.Key) {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "该类型的数据项key已存在")
	}

	item := model.DataDictionaryItem{
		Key:         req.Key,
		Type:        req.Type,
		Name:        req.Name,
		Value:       req.Value,
		Sort:        req.Sort,
		Description: req.Description,
	}

	if err := l.svcCtx.PowerX.DataDictionary.CreateDataDictionaryItem(l.ctx, &item); err != nil {
		return nil, err
	}

	return &types.CreateDictionaryItemReply{
		Key:  item.Key,
		Type: item.Type,
	}, nil
}
