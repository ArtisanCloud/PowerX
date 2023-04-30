package product

import (
	"PowerX/internal/model/custom/product"
	product2 "PowerX/internal/uc/custom/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListServiceSpecificsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListServiceSpecificsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListServiceSpecificsLogic {
	return &ListServiceSpecificsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListServiceSpecificsLogic) ListServiceSpecifics(req *types.ListServiceSpecificPageRequest) (resp *types.ListServiceSpecificPageReply, err error) {

	page, err := l.svcCtx.Custom.ServiceSpecific.FindManyServiceSpecifics(l.ctx, &product2.FindServiceSpecificOption{
		OrderBy: req.OrderBy,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})

	if err != nil {
		return nil, err
	}

	list := TransferServicesToServicesReply(page.List)
	return &types.ListServiceSpecificPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil

}

func TransferServicesToServicesReply(services []*product.ServiceSpecific) []*types.ServiceSpecific {
	serviceSpecificsReply := []*types.ServiceSpecific{}
	for _, serviceSpecific := range services {
		serviceSpecificReply := TransferServiceSpecificToServiceSpecificReply(serviceSpecific)
		serviceSpecificsReply = append(serviceSpecificsReply, serviceSpecificReply)
	}
	return serviceSpecificsReply
}
