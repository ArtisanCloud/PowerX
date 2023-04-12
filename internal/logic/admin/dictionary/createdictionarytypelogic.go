package dictionary

import (
	"PowerX/internal/model"
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
	typ := model.DataDictionaryType{
		Type:        req.Type,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := l.svcCtx.PowerX.DataDictionaryUserCase.CreateDataDictionaryType(l.ctx, &typ); err != nil {
		return nil, err
	}

	return &types.CreateDictionaryTypeReply{
		Type: typ.Type,
	}, nil
}
