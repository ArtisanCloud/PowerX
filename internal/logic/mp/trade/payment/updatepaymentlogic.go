package payment

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdatePaymentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdatePaymentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdatePaymentLogic {
	return &UpdatePaymentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdatePaymentLogic) UpdatePayment(req *types.UpdatePaymentRequest) (resp *types.UpdatePaymentReply, err error) {
	// todo: add your logic here and delete this line

	return
}
