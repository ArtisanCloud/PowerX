package productspecific

import (
	product2 "PowerX/internal/model/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetProductSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetProductSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetProductSpecificLogic {
	return &GetProductSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetProductSpecificLogic) GetProductSpecific(req *types.GetProductSpecificRequest) (resp *types.GetProductSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}

func TransformProductSpecificToProductSpecificReply(specific *product2.ProductSpecific) (specificReply *types.ProductSpecific) {
	if specific == nil {
		return nil
	}

	return &types.ProductSpecific{
		Id:              specific.Id,
		Name:            specific.Name,
		SpecificOptions: TransformSpecificOptionsToReply(specific.Options),
	}
}

func TransformSpecificOptionsToReply(options []*product2.SpecificOption) (optionsReply []*types.SpecificOption) {

	optionsReply = []*types.SpecificOption{}
	for _, option := range options {
		optionsReply = append(optionsReply, TransformSpecificOptionToReply(option))

	}

	return optionsReply
}

func TransformSpecificOptionToReply(option *product2.SpecificOption) (optionReply *types.SpecificOption) {
	if option == nil {
		return nil
	}

	return &types.SpecificOption{
		Id:          option.Id,
		Name:        option.Name,
		IsActivated: option.IsActivated,
	}
}
