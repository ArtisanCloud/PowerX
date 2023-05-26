package mediaresource

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListMediaResourcesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListMediaResourcesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMediaResourcesLogic {
	return &ListMediaResourcesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListMediaResourcesLogic) ListMediaResources(req *types.ListMediaResourcesPageRequest) (resp *types.ListMediaResourcesPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
