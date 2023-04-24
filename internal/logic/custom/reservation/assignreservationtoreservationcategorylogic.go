package reservation

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignReservationToReservationCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignReservationToReservationCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignReservationToReservationCategoryLogic {
	return &AssignReservationToReservationCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignReservationToReservationCategoryLogic) AssignReservationToReservationCategory(req *types.AssignReservationToReservationCategoryRequest) (resp *types.AssignReservationToReservationCategoryReply, err error) {
	// todo: add your logic here and delete this line

	return
}
