package membership

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMembershipLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetMembershipLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMembershipLogic {
	return &GetMembershipLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetMembershipLogic) GetMembership(req *types.GetMembershipRequest) (resp *types.GetMembershipReply, err error) {
	// todo: add your logic here and delete this line

	return
}
