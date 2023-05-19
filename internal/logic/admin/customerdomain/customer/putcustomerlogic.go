package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutCustomerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutCustomerLogic {
	return &PutCustomerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutCustomerLogic) PutCustomer(req *types.PutCustomerRequest) (resp *types.PutCustomerReply, err error) {
	// todo: add your logic here and delete this line

	return
}
