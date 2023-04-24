package reservation

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateReservationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateReservationLogic {
	return &CreateReservationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateReservationLogic) CreateReservation(req *types.CreateReservationRequest) (resp *types.CreateReservationReply, err error) {
	// todo: add your logic here and delete this line

	return
}
