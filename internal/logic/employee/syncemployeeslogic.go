package employee

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncEmployeesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSyncEmployeesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncEmployeesLogic {
	return &SyncEmployeesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SyncEmployeesLogic) SyncEmployees(req *types.SyncEmployeesRequest) (resp *types.SyncEmployeesReply, err error) {
	syncCase := req.Source + "-" + req.Target
	switch syncCase {
	case "wework-system":
		l.svcCtx.UC.WeWork.FetchDepartments(l.ctx)
		l.svcCtx.UC.WeWork.FetchEmployees(l.ctx)
		l.svcCtx.UC.WeWork.SyncDepartmentsToSystem(l.ctx)
		l.svcCtx.UC.WeWork.SyncEmployeeToSystem(l.ctx)
		//l.svcCtx.UC.SyncWeWork.SyncDepartmentsLeadersToSystem(l.ctx)
		return &types.SyncEmployeesReply{
			Status: true,
		}, nil
	}
	return &types.SyncEmployeesReply{
		Status: false,
	}, errorx.WithCause(errorx.ErrBadRequest, "违规同步类型")
}
