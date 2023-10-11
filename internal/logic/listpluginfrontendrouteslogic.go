package logic

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
	routes := l.svcCtx.Plugin.ListFrontendRoutes()
	var routesList []types.PluginWebRoutes
	for _, route := range routes {
		routesList = append(routesList, types.PluginWebRoutes{
			Name: route.Name,
			Path: route.Path,
			Meta: types.PluginWebRouteMeta{
				Locale: route.Meta.Locale,
				Icon:   route.Meta.Icon,
			},
		})
	}
	return &types.ListPluginFrontendRoutesReply{
		Routes: routesList,
	}, nil
}
