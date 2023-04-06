package dictionary

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetDictionaryTypesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetDictionaryTypesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDictionaryTypesLogic {
	return &GetDictionaryTypesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetDictionaryTypesLogic) GetDictionaryTypes(req *types.GetDictionaryTypesRequest) (resp *types.GetDictionaryTypesReply, err error) {
	// todo: add your logic here and delete this line

	return
}
