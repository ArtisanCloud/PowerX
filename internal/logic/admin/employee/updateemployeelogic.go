package employee

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateEmployeeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateEmployeeLogic {
	return &UpdateEmployeeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateEmployeeLogic) UpdateEmployee(req *types.UpdateEmployeeRequest) (resp *types.UpdateEmployeeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
