package dictionary

import (
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
	// todo: add your logic here and delete this line

	return
}
