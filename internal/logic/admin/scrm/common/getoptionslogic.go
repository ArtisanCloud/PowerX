package common

import (
	"context"
	"fmt"

	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx/scrm/wechat"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetOptionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOptionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOptionsLogic {
	return &GetOptionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetOptionsLogic) GetOptions(req *types.GetOptionsRequest) (resp *types.GetOptionsReply, err error) {
	switch req.Type {
	case "customer-external":
		result, err := l.svcCtx.PowerX.SCRM.Wechat.FindManyWeWorkCustomerPage(l.ctx, &types.PageOption[wechat.FindManyWechatCustomerOption]{
			PageIndex: 1,
			PageSize:  10,
			Option: wechat.FindManyWechatCustomerOption{
				Name: req.Search,
			},
		}, 0)
		if err != nil {
			return nil, err
		}
		options := make([]map[string]any, len(result.List))
		for i, v := range result.List {
			options[i] = map[string]any{
				"label": v.Name,
				"value": v.ExternalUserId,
			}
		}
		resp.Options = options
	default:
		err = fmt.Errorf("invalid type: %s", req.Type)
	}
	return resp, err
}
