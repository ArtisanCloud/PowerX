package productstatistics

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutProductStatisticsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutProductStatisticsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutProductStatisticsLogic {
	return &PutProductStatisticsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutProductStatisticsLogic) PutProductStatistics(req *types.PutProductStatisticsRequest) (resp *types.PutProductStatisticsReply, err error) {
	// todo: add your logic here and delete this line

	return
}
