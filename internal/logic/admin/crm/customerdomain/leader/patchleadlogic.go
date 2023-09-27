package leader

import (
	"PowerX/internal/model/crm/customerdomain"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchLeadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchLeadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchLeadLogic {
	return &PatchLeadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchLeadLogic) PatchLead(req *types.PatchLeadRequest) (resp *types.PatchLeadReply, err error) {

	mdlLead := &customerdomain.Lead{
		Name:        req.Name,
		Email:       req.Email,
		InviterId:   req.InviterId,
		Source:      req.Source,
		Type:        req.Type,
		IsActivated: req.IsActivated,
	}

	// 更新产品对象
	err = l.svcCtx.PowerX.Lead.UpdateLead(l.ctx, req.LeadId, mdlLead)

	return &types.PatchLeadReply{
		Lead: TransformLeadToReply(l.svcCtx, mdlLead),
	}, err

}
