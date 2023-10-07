package productstatistics

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchProductStatisticsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchProductStatisticsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchProductStatisticsLogic {
	return &PatchProductStatisticsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchProductStatisticsLogic) PatchProductStatistics(req *types.PatchProductStatisticsRequest) (resp *types.PatchProductStatisticsReply, err error) {
	// todo: add your logic here and delete this line

	return
}
