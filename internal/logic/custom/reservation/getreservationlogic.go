package reservation

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetReservationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetReservationLogic {
	return &GetReservationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetReservationLogic) GetReservation(req *types.GetReservationRequest) (resp *types.GetReservationReply, err error) {
	// todo: add your logic here and delete this line

	return
}
