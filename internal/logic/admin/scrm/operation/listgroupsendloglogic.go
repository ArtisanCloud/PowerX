package operation

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListGroupSendLogLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListGroupSendLogLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListGroupSendLogLogic {
	return &ListGroupSendLogLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListGroupSendLogLogic) ListGroupSendLog(req *types.ListGroupSendLogRequest) (resp *types.ListGroupSendLogReply, err error) {
	// todo: add your logic here and delete this line

	return
}
