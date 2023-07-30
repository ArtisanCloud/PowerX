package leader

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutLeadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutLeadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutLeadLogic {
	return &PutLeadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutLeadLogic) PutLead(req *types.PutLeadRequest) (resp *types.PutLeadReply, err error) {
	mdlLead := TransformLeadRequestToLead(&(req.Lead))

	// 更新产品对象
	err = l.svcCtx.PowerX.Lead.UpdateLead(l.ctx, req.LeadId, mdlLead)

	return &types.PutLeadReply{
		Lead: TransformLeadToLeadReply(l.svcCtx, mdlLead),
	}, err
}
