package dictionary

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListDictionaryPageTypesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListDictionaryPageTypesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListDictionaryPageTypesLogic {
	return &ListDictionaryPageTypesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListDictionaryPageTypesLogic) ListDictionaryPageTypes(req *types.ListDictionaryTypesPageRequest) (resp *types.ListDictionaryTypesPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
