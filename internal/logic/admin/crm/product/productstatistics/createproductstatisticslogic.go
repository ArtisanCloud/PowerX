package productstatistics

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateProductStatisticsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateProductStatisticsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateProductStatisticsLogic {
	return &CreateProductStatisticsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateProductStatisticsLogic) CreateProductStatistics(req *types.CreateProductStatisticsRequest) (resp *types.CreateProductStatisticsReply, err error) {
	// todo: add your logic here and delete this line

	return
}
