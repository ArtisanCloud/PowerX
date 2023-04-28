package lead

import (
	"PowerX/internal/model/customerdomain"
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

	lead := &customerdomain.Lead{
		Name:        req.Name,
		Mobile:      req.Mobile,
		Email:       req.Email,
		InviterId:   req.InviterId,
		Source:      req.Source,
		Type:        req.Type,
		IsActivated: req.IsActivated,
	}

	l.svcCtx.PowerX.Lead.CreateLead(l.ctx, lead)

	return &types.CreateLeadReply{
		lead.Id,
	}, nil

}
