package dictionary

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateDictionaryTypeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateDictionaryTypeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateDictionaryTypeLogic {
	return &CreateDictionaryTypeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateDictionaryTypeLogic) CreateDictionaryType(req *types.CreateDictionaryTypeRequest) (resp *types.CreateDictionaryTypeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
