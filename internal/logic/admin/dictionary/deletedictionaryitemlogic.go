package dictionary

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteDictionaryItemLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteDictionaryItemLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteDictionaryItemLogic {
	return &DeleteDictionaryItemLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteDictionaryItemLogic) DeleteDictionaryItem(req *types.DeleteDictionaryItemRequest) (resp *types.DeleteDictionaryItemReply, err error) {
	return &types.DeleteDictionaryItemReply{
		Key:  req.Key,
		Type: req.Type,
	}, nil
}
