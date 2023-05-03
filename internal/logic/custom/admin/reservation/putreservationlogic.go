package reservation

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutReservationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutReservationLogic {
	return &PutReservationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutReservationLogic) PutReservation(req *types.PutReservationRequest) (resp *types.PutReservationReply, err error) {
	// todo: add your logic here and delete this line

	return
}
