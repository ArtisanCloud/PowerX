package dictionary

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteDictionaryTypeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteDictionaryTypeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteDictionaryTypeLogic {
	return &DeleteDictionaryTypeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteDictionaryTypeLogic) DeleteDictionaryType(req *types.DeleteDictionaryTypeRequest) (resp *types.DeleteDictionaryTypeReply, err error) {
	if err := l.svcCtx.PowerX.DataDictionaryUserCase.DeleteDataDictionaryType(l.ctx, req.Type); err != nil {
		return nil, err
	}

	return &types.DeleteDictionaryTypeReply{
		Type: req.Type,
	}, nil
}
