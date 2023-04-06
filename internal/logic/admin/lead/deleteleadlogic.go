package lead

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteLeadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteLeadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteLeadLogic {
	return &DeleteLeadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteLeadLogic) DeleteLead(req *types.DeleteLeadRequest) (resp *types.DeleteLeadReply, err error) {
	// todo: add your logic here and delete this line

	return
}
