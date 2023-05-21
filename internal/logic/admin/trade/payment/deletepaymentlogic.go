package payment

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeletePaymentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeletePaymentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeletePaymentLogic {
	return &DeletePaymentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeletePaymentLogic) DeletePayment(req *types.DeletePaymentRequest) (resp *types.DeletePaymentReply, err error) {
	// todo: add your logic here and delete this line

	return
}
