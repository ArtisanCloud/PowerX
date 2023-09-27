package auth

import (
	"PowerX/internal/logic/admin/crm/customerdomain/customer"
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx/crm/customerdomain"
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserInfoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserInfoLogic {
	return &GetUserInfoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserInfoLogic) GetUserInfo() (resp *types.GetUserInfoReplyToWeb, err error) {

	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	customer := customer.TransformCustomerToReply(l.svcCtx, authCustomer)
	customer.AccountId = fmt.Sprintf("%d", customer.Id)
	return &types.GetUserInfoReplyToWeb{
		Customer: customer,
	}, nil
}
