package openapi

import (
	"PowerX/internal/uc"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateEchoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// Create a new echo message
func NewCreateEchoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateEchoLogic {
	return &CreateEchoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateEchoLogic) CreateEcho(req *types.CreateEchoRequest) (resp *types.CreateEchoResponse, err error) {
	vAuthPlatform := l.ctx.Value(uc.AuthPlatformKey)
	authCustomer := vAuthPlatform.(string)

	return &types.CreateEchoResponse{
		Response: "hello:" + authCustomer,
	}, nil
}
