package token

import (
	customerdomain2 "PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/crm/trade"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx/crm/customerdomain"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCustomerTokenBalanceLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetCustomerTokenBalanceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCustomerTokenBalanceLogic {
	return &GetCustomerTokenBalanceLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetCustomerTokenBalanceLogic) GetCustomerTokenBalance() (resp *types.GetCustomerBalanceReply, err error) {
	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	balance, unusedTicketsCount, err := l.svcCtx.PowerX.Token.CheckTokenBalanceIsEnough(l.ctx, authCustomer)
	if err != nil {
		return nil, err
	}
	balanceReply := TransformBalanceToReplyForMP(balance)
	balanceReply.UnusedTicketsCount = unusedTicketsCount
	return &types.GetCustomerBalanceReply{
		TokenBalance: balanceReply,
	}, nil
}

func TransformBalanceToReplyForMP(balance *trade.TokenBalance) *types.TokenBalance {
	return &types.TokenBalance{
		Id:      balance.Id,
		Balance: balance.Balance,
		Usage:   balance.Usage,
	}
}
