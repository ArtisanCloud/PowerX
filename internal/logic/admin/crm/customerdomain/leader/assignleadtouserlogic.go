package leader

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignLeadToUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignLeadToUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignLeadToUserLogic {
	return &AssignLeadToUserLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignLeadToUserLogic) AssignLeadToUser(req *types.AssignLeadToUserRequest) (resp *types.AssignLeadToUserReply, err error) {
	// todo: add your logic here and delete this line

	return
}
