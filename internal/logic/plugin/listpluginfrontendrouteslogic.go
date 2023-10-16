package plugin

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListPluginFrontendRoutesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListPluginFrontendRoutesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPluginFrontendRoutesLogic {
	return &ListPluginFrontendRoutesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListPluginFrontendRoutesLogic) ListPluginFrontendRoutes() (resp *types.ListPluginFrontendRoutesReply, err error) {
	// todo: add your logic here and delete this line

	return
}
