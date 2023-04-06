package dictionary

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateDictionaryTypeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateDictionaryTypeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateDictionaryTypeLogic {
	return &UpdateDictionaryTypeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateDictionaryTypeLogic) UpdateDictionaryType(req *types.UpdateDictionaryTypeRequest) (resp *types.UpdateDictionaryTypeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
