package leader

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateLeadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateLeadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateLeadLogic {
	return &CreateLeadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateLeadLogic) CreateLead(req *types.CreateLeadRequest) (resp *types.CreateLeadReply, err error) {
	// todo: add your logic here and delete this line

	return
}
