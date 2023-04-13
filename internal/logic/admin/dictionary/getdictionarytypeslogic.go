package dictionary

import (
	"PowerX/internal/uc/powerx"
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
	page, err := l.svcCtx.PowerX.DataDictionaryUserCase.FindManyDataDictionaryType(l.ctx, &powerx.FindManyDataDictTypeOption{
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := make([]types.DictionaryType, 0, len(page.List))
	for _, item := range page.List {
		list = append(list, types.DictionaryType{
			Type:        item.Type,
			Name:        item.Name,
			Description: item.Description,
		})
	}

	return &types.GetDictionaryTypesReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil
}
