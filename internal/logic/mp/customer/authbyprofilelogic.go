package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AuthByProfileLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAuthByProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AuthByProfileLogic {
	return &AuthByProfileLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AuthByProfileLogic) AuthByProfile() (resp *types.MPCustomerLoginAuthReply, err error) {
	// todo: add your logic here and delete this line

	return
}
