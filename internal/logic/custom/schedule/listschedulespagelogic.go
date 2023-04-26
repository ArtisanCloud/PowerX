package schedule

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListSchedulesPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListSchedulesPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSchedulesPageLogic {
	return &ListSchedulesPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListSchedulesPageLogic) ListSchedulesPage(req *types.ListSchedulesPageRequest) (resp *types.ListSchedulesPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
