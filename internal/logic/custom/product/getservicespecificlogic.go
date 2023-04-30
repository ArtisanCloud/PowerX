package product

import (
	"PowerX/internal/model/custom/product"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetServiceSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetServiceSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetServiceSpecificLogic {
	return &GetServiceSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetServiceSpecificLogic) GetServiceSpecific(req *types.GetServiceSpecificRequest) (resp *types.GetServiceSpecificReply, err error) {
	mdlServiceSpecific, err := l.svcCtx.Custom.ServiceSpecific.GetServiceSpecific(l.ctx, req.ServiceSpecificId)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetServiceSpecificReply{
		ServiceSpecific: TransferServiceSpecificToServiceSpecificReply(mdlServiceSpecific),
	}, nil
}

func TransferServiceSpecificToServiceSpecificReply(serviceSpecific *product.ServiceSpecific) *types.ServiceSpecific {
	return &types.ServiceSpecific{
		Id:                serviceSpecific.Id,
		ParentId:          serviceSpecific.ParentId,
		ProductId:         serviceSpecific.ProductId,
		IsFree:            serviceSpecific.IsFree,
		Name:              serviceSpecific.Name,
		Duration:          serviceSpecific.Duration,
		MandatoryDuration: serviceSpecific.MandatoryDuration,
		CreatedAt:         serviceSpecific.CreatedAt.String(),
		Children:          TransferServiceSpecificToServiceSpecificChildrenReply(serviceSpecific.Children),
		Product: &types.SSRefProduct{
			Id:            serviceSpecific.Product.Id,
			Name:          serviceSpecific.Product.Name,
			Type:          serviceSpecific.Product.Type,
			Plan:          serviceSpecific.Product.Plan,
			CanSellOnline: serviceSpecific.Product.CanSellOnline,
			Description:   serviceSpecific.Product.Description,
			CoverURL:      serviceSpecific.Product.CoverURL,
		},
		//Stores: []&types.SSRefStore{
		//	Id:            serviceSpecific.Store.Id,
		//	Name:          serviceSpecific.Store.Name,
		//	EmployeeId:    serviceSpecific.Store.EmployeeId,
		//	ContactNumber: serviceSpecific.Store.ContactNumber,
		//	CoverURL:      serviceSpecific.Store.CoverURL,
		//	Address:       serviceSpecific.Store.Address,
		//	Longitude:     serviceSpecific.Store.Longitude,
		//	Latitude:      serviceSpecific.Store.Latitude,
		//	StartWork:     serviceSpecific.Store.StartWork,
		//	EndWork:       serviceSpecific.Store.EndWork,
		//},
	}
}

func TransferServiceSpecificToServiceSpecificChildrenReply(children []*product.ServiceSpecific) []*types.ChildSpecific {
	childrenReply := []*types.ChildSpecific{}
	for _, serviceSpecific := range children {
		childReply := TransferServiceSpecificToChildReply(serviceSpecific)
		childrenReply = append(childrenReply, childReply)
	}
	return childrenReply
}

func TransferServiceSpecificToChildReply(child *product.ServiceSpecific) *types.ChildSpecific {
	return &types.ChildSpecific{
		Id:                child.Id,
		IsFree:            child.IsFree,
		Name:              child.Name,
		Duration:          child.Duration,
		MandatoryDuration: child.MandatoryDuration,
	}
}
