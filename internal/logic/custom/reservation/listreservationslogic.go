package reservation

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListReservationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListReservationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListReservationsLogic {
	return &ListReservationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListReservationsLogic) ListReservations(req *types.ListReservationsPageRequest) (resp *types.ListReservationsPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
