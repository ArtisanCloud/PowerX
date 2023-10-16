package plugin

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterPluginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterPluginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterPluginLogic {
	return &RegisterPluginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RegisterPluginLogic) RegisterPlugin(req *types.RegisterPluginRequest) (resp *types.RegisterPluginReply, err error) {
	// todo: add your logic here and delete this line

	return
}
