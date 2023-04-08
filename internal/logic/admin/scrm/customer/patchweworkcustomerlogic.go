package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchWeWorkCustomerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchWeWorkCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchWeWorkCustomerLogic {
	return &PatchWeWorkCustomerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchWeWorkCustomerLogic) PatchWeWorkCustomer(req *types.PatchWeWorkCustomerRequest) (resp *types.PatchWeWorkCustomerReply, err error) {
	// todo: add your logic here and delete this line

	return
}
