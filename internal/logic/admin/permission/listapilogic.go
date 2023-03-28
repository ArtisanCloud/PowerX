package permission

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListAPILogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAPILogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAPILogic {
	return &ListAPILogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListAPILogic) ListAPI(req *types.ListAPIRequest) (resp *types.ListAPIReply, err error) {
	apis := l.svcCtx.PowerX.Auth.FindAllAPI(l.ctx)

	var apiList []types.AdminAPI
	for _, api := range apis {
		apiList = append(apiList, types.AdminAPI{
			Id:     api.ID,
			API:    api.API,
			Method: api.Method,
			Name:   api.Name,
			Desc:   api.Desc,
		})
	}

	return &types.ListAPIReply{
		List: apiList,
	}, nil
}
