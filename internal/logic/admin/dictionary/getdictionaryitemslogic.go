package dictionary

import (
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
	// todo: add your logic here and delete this line

	return
}
