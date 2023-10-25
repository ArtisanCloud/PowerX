package plugin

import (
	"PowerX/pkg/pluginx"
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
	var routes []pluginx.BackendRoute
	for _, route := range req.Routes {
		routes = append(routes, pluginx.BackendRoute{
			Method: route.Method,
			Path:   route.Path,
		})
	}
	l.Infof("plugin register request: %v", req)
	etc, err := l.svcCtx.Plugin.Register(req.Name, req.Addr, routes)
	if err != nil {
		return nil, err
	}
	l.Infof("plugin %s registered", req.Name)

	return &types.RegisterPluginReply{
		Etc: etc,
	}, nil
}
