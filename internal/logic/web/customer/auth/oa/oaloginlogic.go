package oa

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type OALoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewOALoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *OALoginLogic {
	return &OALoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *OALoginLogic) OALogin(req *types.OACustomerLoginRequest) (resp *types.OACustomerLoginAuthReply, err error) {

	return &types.OACustomerLoginAuthReply{
		OpenId: req.Code,
	}, nil
}
