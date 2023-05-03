package reservation

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchReservationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchReservationLogic {
	return &PatchReservationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchReservationLogic) PatchReservation(req *types.PatchReservationRequest) (resp *types.PatchReservationReply, err error) {
	// todo: add your logic here and delete this line

	return
}
