package productspecific

import (
	product2 "PowerX/internal/model/crm/product"
	"context"
	"time"

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

	specific := TransformRequestToProductSpecific(req.ProductSpecific)

	err = l.svcCtx.PowerX.ProductSpecific.CreateProductSpecific(l.ctx, specific)

	if err != nil {
		return nil, err
	}

	return &types.CreateProductSpecificReply{
		ProductSpecificId: specific.Id,
	}, nil
}

func TransformRequestToProductSpecific(specificRequest types.ProductSpecific) *product2.ProductSpecific {

	if specificRequest.ProductId <= 0 || specificRequest.Name == "" {
		return nil
	}
	specific := &product2.ProductSpecific{
		ProductId: specificRequest.ProductId,
		Name:      specificRequest.Name,
		Options:   TransformRequestToSpecificOptions(specificRequest.SpecificOptions),
	}

	if specificRequest.Id > 0 {
		specific.Id = specificRequest.Id
	}
	return specific
}

func TransformRequestToSpecificOptions(optionsRequest []*types.SpecificOption) (options []*product2.SpecificOption) {
	options = []*product2.SpecificOption{}
	for _, optionRequest := range optionsRequest {
		option := TransformRequestToSpecificOption(optionRequest)
		if option != nil {
			options = append(options, option)
		}
	}
	return options
}

func TransformRequestToSpecificOption(optionRequest *types.SpecificOption) (option *product2.SpecificOption) {
	if optionRequest == nil {
		return nil
	}

	if optionRequest.ProductSpecificId < 0 || optionRequest.Name == "" {
		return nil
	}

	option = &product2.SpecificOption{
		Name:        optionRequest.Name,
		IsActivated: optionRequest.IsActivated,
	}
	// 前端如果是新建的选项，那么不会有 ProductSpecificId， 但仍需要新建
	if optionRequest.ProductSpecificId > 0 {
		option.ProductSpecificId = optionRequest.ProductSpecificId
	}
	// 更新已存在的选项
	if optionRequest.Id > 0 {
		option.Id = optionRequest.Id
		option.UpdatedAt = time.Now()
	}
	return option
}
