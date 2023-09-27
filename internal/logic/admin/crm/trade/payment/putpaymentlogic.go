package payment

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutPaymentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutPaymentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutPaymentLogic {
	return &PutPaymentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutPaymentLogic) PutPayment(req *types.PutPaymentRequest) (resp *types.PutPaymentReply, err error) {
	// todo: add your logic here and delete this line

	return
}
