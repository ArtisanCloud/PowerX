package dictionary

import (
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
	// todo: add your logic here and delete this line

	return
}
