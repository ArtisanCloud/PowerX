package payment

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchPaymentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchPaymentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchPaymentLogic {
	return &PatchPaymentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchPaymentLogic) PatchPayment(req *types.PatchPaymentRequest) (resp *types.PatchPaymentReply, err error) {
	// todo: add your logic here and delete this line

	return
}
