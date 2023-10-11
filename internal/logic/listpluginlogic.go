package logic

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListPluginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListPluginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPluginLogic {
	return &ListPluginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListPluginLogic) ListPlugin(req *types.ListPluginRequest) (resp *types.ListPluginReply, err error) {
	// todo: add your logic here and delete this line

	return
}
