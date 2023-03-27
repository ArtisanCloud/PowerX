package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchCustomerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchCustomerLogic {
	return &PatchCustomerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchCustomerLogic) PatchCustomer(req *types.PatchCustomerRequest) (resp *types.PatchCustomerReply, err error) {
	// todo: add your logic here and delete this line

	return
}
