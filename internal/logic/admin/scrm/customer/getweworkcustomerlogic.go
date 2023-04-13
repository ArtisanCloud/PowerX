package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetWeWorkCustomerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetWeWorkCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetWeWorkCustomerLogic {
	return &GetWeWorkCustomerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetWeWorkCustomerLogic) GetWeWorkCustomer(req *types.GetWeWorkCustomerRequest) (resp *types.GetWeWorkCustomerReply, err error) {
	// todo: add your logic here and delete this line

	return
}
