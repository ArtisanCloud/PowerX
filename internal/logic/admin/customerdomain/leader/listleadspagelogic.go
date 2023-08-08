package leader

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/uc/powerx/customerdomain"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListLeadsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListLeadsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListLeadsPageLogic {
	return &ListLeadsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListLeadsPageLogic) ListLeadsPage(req *types.ListLeadsPageRequest) (resp *types.ListLeadsPageReply, err error) {

	page, err := l.svcCtx.PowerX.Lead.FindManyLeads(l.ctx, &customerdomain.FindManyLeadsOption{
		LikeName:   req.LikeName,
		LikeMobile: req.LikeMobile,
		Statuses:   req.Statuses,
		Sources:    req.Sources,
		OrderBy:    req.OrderBy,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformLeadsToReply(l.svcCtx, page.List)
	return &types.ListLeadsPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil

}

func TransformLeadsToReply(svcCtx *svc.ServiceContext, leads []*customerdomain2.Lead) []types.Lead {
	leadsReply := []types.Lead{}
	for _, lead := range leads {
		leadReply := TransformLeadToReply(svcCtx, lead)
		leadsReply = append(leadsReply, *leadReply)

	}
	return leadsReply
}
