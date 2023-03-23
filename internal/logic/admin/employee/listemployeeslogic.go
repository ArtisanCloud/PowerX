package employee

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListEmployeesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListEmployeesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListEmployeesLogic {
	return &ListEmployeesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListEmployeesLogic) ListEmployees(req *types.ListEmployeesRequest) (resp *types.ListEmployeesReply, err error) {
	// todo: add your logic here and delete this line

	return
}
