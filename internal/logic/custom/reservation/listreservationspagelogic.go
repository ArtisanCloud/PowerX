package reservation

import (
	reservationcenter2 "PowerX/internal/model/custom/reservationcenter"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListReservationsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListReservationsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListReservationsPageLogic {
	return &ListReservationsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListReservationsPageLogic) ListReservationsPage(req *types.ListReservationsPageRequest) (resp *types.ListReservationsPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}

func TransformReservationsToReservationsReply(reservations []*reservationcenter2.Reservation) []*types.Reservation {

	list := make([]*types.Reservation, 0, len(reservations))
	for _, item := range reservations {
		list = append(list, TransformReservationToReservationReply(item))
	}

	return list
}
