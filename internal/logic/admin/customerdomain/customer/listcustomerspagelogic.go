package customer

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/uc/powerx/customerdomain"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListCustomersPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListCustomersPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListCustomersPageLogic {
	return &ListCustomersPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListCustomersPageLogic) ListCustomersPage(req *types.ListCustomersPageRequest) (resp *types.ListCustomersPageReply, err error) {

	page, err := l.svcCtx.PowerX.Customer.FindManyCustomers(l.ctx, &customerdomain.FindManyCustomersOption{
		LikeName:   req.LikeName,
		LikeMobile: req.LikeMobile,
		Statuses:   req.Statuses,
		Sources:    req.Sources,
		OrderBy:    req.OrderBy,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformCustomersToCustomersReply(l.svcCtx, page.List)
	return &types.ListCustomersPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil

}

func TransformCustomersToCustomersReply(svcCtx *svc.ServiceContext, customers []*customerdomain2.Customer) []types.Customer {
	customersReply := []types.Customer{}
	for _, customer := range customers {
		customerReply := TransformCustomerToCustomerReply(svcCtx, customer)
		customersReply = append(customersReply, *customerReply)

	}
	return customersReply
}
