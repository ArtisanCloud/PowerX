package contact

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkEmployeeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListWeWorkEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkEmployeeLogic {
	return &ListWeWorkEmployeeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListWeWorkEmployeeLogic) ListWeWorkEmployee(req *types.ListWeWorkEmployeeReqeust) (resp *types.ListWeWorkEmployeeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
