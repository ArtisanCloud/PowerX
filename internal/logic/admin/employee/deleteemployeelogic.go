package employee

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteEmployeeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteEmployeeLogic {
	return &DeleteEmployeeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteEmployeeLogic) DeleteEmployee(req *types.DeleteEmployeeRequest) (resp *types.DeleteEmployeeReply, err error) {
	err = l.svcCtx.PowerX.Organization.DeleteEmployeeById(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &types.DeleteEmployeeReply{
		Id: req.Id,
	}, nil
}
