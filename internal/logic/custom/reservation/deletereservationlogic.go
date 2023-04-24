package reservation

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteReservationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteReservationLogic {
	return &DeleteReservationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteReservationLogic) DeleteReservation(req *types.DeleteReservationRequest) (resp *types.DeleteReservationReply, err error) {
	// todo: add your logic here and delete this line

	return
}
