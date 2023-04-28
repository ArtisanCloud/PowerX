package lead

import (
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLeadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetLeadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLeadLogic {
	return &GetLeadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetLeadLogic) GetLead(req *types.GetLeadReqeuest) (resp *types.GetLeadReply, err error) {
	mdlLead, err := l.svcCtx.PowerX.Lead.GetLead(l.ctx, req.Id)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetLeadReply{
		Lead: TransformLeadToLeadReply(mdlLead),
	}, nil
}

func TransformLeadToLeadReply(mdlLead *customerdomain.Lead) (leadReply *types.Lead) {

	var inviter *types.LeadInviter
	if mdlLead.Inviter != nil {
		inviter = &types.LeadInviter{
			Name:   mdlLead.Inviter.Name,
			Mobile: mdlLead.Inviter.Mobile,
			Email:  mdlLead.Inviter.Email,
		}
	}

	return &types.Lead{
		Id:          mdlLead.Id,
		Name:        mdlLead.Name,
		Mobile:      mdlLead.Mobile,
		Email:       mdlLead.Email,
		InviterId:   mdlLead.InviterId,
		Source:      mdlLead.Source,
		Type:        mdlLead.Type,
		IsActivated: mdlLead.IsActivated,
		CreatedAt:   mdlLead.CreatedAt.String(),
		Inviter:     inviter,
		LeadExternalId: &types.LeadExternalId{
			OpenIdInMiniProgram:           mdlLead.OpenIdInMiniProgram,
			OpenIdInWeChatOfficialAccount: mdlLead.OpenIdInWeChatOfficialAccount,
			OpenIdInWeCom:                 mdlLead.OpenIdInWeCom,
		},
	}

}
