package membership

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCustomerMembershipLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetCustomerMembershipLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCustomerMembershipLogic {
	return &GetCustomerMembershipLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetCustomerMembershipLogic) GetCustomerMembership(req *types.GetCustomerMembershipByTypeRequest) (resp *types.GetCustomerMembershipByTypeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
