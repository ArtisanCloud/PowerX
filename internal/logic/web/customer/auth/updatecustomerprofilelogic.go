package auth

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateCustomerProfileLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateCustomerProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateCustomerProfileLogic {
	return &UpdateCustomerProfileLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateCustomerProfileLogic) UpdateCustomerProfile(req *types.UpdateCustomerProfileRequest) (resp *types.UpdateCustomerProfileReply, err error) {
	// todo: add your logic here and delete this line

	return
}
