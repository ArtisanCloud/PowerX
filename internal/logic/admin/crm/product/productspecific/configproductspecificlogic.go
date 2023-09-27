package productspecific

import (
	product2 "PowerX/internal/model/crm/product"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ConfigProductSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewConfigProductSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfigProductSpecificLogic {
	return &ConfigProductSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ConfigProductSpecificLogic) ConfigProductSpecific(req *types.ConfigProductSpecificRequest) (resp *types.ConfigProductSpecificReply, err error) {

	specifics := TransformProductSpecificsRequestToProductSpecifics(req.ProductSpecifics)
	options := GetOptionsFromProductSpecificsRequest(specifics)
	//fmt.Dump(specifics)
	err = l.svcCtx.PowerX.ProductSpecific.ConfigProductSpecific(l.ctx, specifics, options)
	//specifics, err = l.svcCtx.PowerX.ProductSpecific.UpsertProductSpecifics(l.ctx, specifics)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	// ReFact product skus
	product, err := l.svcCtx.PowerX.Product.GetProduct(l.ctx, specifics[0].ProductId)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}
	err = l.svcCtx.PowerX.Product.ReFactSKUs(l.ctx, product)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	return &types.ConfigProductSpecificReply{
		Result: true,
	}, nil
}

func TransformProductSpecificsRequestToProductSpecifics(specificsRequest []types.ProductSpecific) []*product2.ProductSpecific {

	specifics := []*product2.ProductSpecific{}
	for _, specificRequest := range specificsRequest {
		specifics = append(specifics, TransformRequestToProductSpecific(specificRequest))
	}

	return specifics
}

func GetOptionsFromProductSpecificsRequest(specifics []*product2.ProductSpecific) []*product2.SpecificOption {
	options := []*product2.SpecificOption{}
	for _, specific := range specifics {
		if len(specific.Options) > 0 {
			for _, option := range specific.Options {
				options = append(options, option)
			}
		}
	}
	return options
}
