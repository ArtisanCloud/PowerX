package productstatistics

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListProductStatisticsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListProductStatisticsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListProductStatisticsPageLogic {
	return &ListProductStatisticsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListProductStatisticsPageLogic) ListProductStatisticsPage(req *types.ListProductStatisticsPageRequest) (resp *types.ListProductStatisticsPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
