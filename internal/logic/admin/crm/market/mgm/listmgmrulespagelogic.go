package mgm

import (
	market2 "PowerX/internal/model/crm/market"
	"PowerX/internal/uc/powerx/crm/market"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListMGMsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListMGMRulesPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMGMsPageLogic {
	return &ListMGMsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListMGMsPageLogic) ListMGMRulesPage(req *types.ListMGMRulesPageRequest) (resp *types.ListMGMRulesPageReply, err error) {
	page, err := l.svcCtx.PowerX.MGM.FindManyMGMRules(l.ctx, &market.FindManyMGMRulesOption{
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformMGMRulesToReply(page.List)
	return &types.ListMGMRulesPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil
}

func TransformMGMRulesToReply(medias []*market2.MGMRule) (mediasReply []*types.MGMRule) {
	mediasReply = []*types.MGMRule{}
	for _, media := range medias {
		mediaReply := TransformMGMRuleToReply(media)
		mediasReply = append(mediasReply, mediaReply)
	}
	return mediasReply

}
