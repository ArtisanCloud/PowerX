package dictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"
	"fmt"

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
	newModel := model.DataDictionaryType{
		Name:        req.Name,
		Description: req.Description,
	}

	if err := l.svcCtx.PowerX.DataDictionary.PatchDataDictionaryType(l.ctx, req.Type, &newModel); err != nil {
		return nil, err
	}

	fmt.Println(newModel)

	return &types.UpdateDictionaryTypeReply{
		DictionaryType: &types.DictionaryType{
			Type:        req.Type,
			Name:        newModel.Name,
			Description: newModel.Description,
		},
	}, nil
}
