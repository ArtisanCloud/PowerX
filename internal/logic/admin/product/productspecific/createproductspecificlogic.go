package productspecific

import (
	product2 "PowerX/internal/model/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateProductSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateProductSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateProductSpecificLogic {
	return &CreateProductSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateProductSpecificLogic) CreateProductSpecific(req *types.CreateProductSpecificRequest) (resp *types.CreateProductSpecificReply, err error) {

	specific := TransformProductSpecificRequestToProductSpecific(req.ProductSpecific)

	err = l.svcCtx.PowerX.ProductSpecific.CreateProductSpecific(l.ctx, specific)

	if err != nil {
		return nil, err
	}

	return &types.CreateProductSpecificReply{
		ProductSpecificId: specific.Id,
	}, nil
}

func TransformProductSpecificRequestToProductSpecific(specificRequest types.ProductSpecific) *product2.ProductSpecific {
	return &product2.ProductSpecific{
		ProductId: specificRequest.ProductId,
		Name:      specificRequest.Name,
		Options:   TransformSpecificOptionsRequestToSpecificOptions(specificRequest.SpecificOptions),
	}
}

func TransformSpecificOptionsRequestToSpecificOptions(optionsRequest []*types.SpecificOption) (options []*product2.SpecificOption) {
	options = []*product2.SpecificOption{}
	for _, optionRequest := range optionsRequest {
		options = append(options, TransformSpecificOptionRequestToSpecificOption(optionRequest))
	}
	return options
}

func TransformSpecificOptionRequestToSpecificOption(optionRequest *types.SpecificOption) (option *product2.SpecificOption) {
	if optionRequest == nil {
		return nil
	}
	return &product2.SpecificOption{
		ProductSpecificId: optionRequest.ProductSpecificId,
		Name:              optionRequest.Name,
		IsActivated:       optionRequest.IsActivated,
	}
}
