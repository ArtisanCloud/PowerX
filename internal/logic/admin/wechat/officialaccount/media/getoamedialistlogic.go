package media

import (
	"PowerX/internal/types/errorx"
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/officialAccount/material/request"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetOAMediaListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOAMediaListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOAMediaListLogic {
	return &GetOAMediaListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetOAMediaListLogic) GetOAMediaList(req *types.GetOAMediaListRequest) (resp *types.GetOAMediaListReply, err error) {

	if req.Count <= 0 {
		req.Count = 10
	}
	res, err := l.svcCtx.PowerX.WechatOA.App.Material.List(l.ctx, &request.RequestMaterialBatchGetMaterial{
		Type:   req.MediaType,
		Offset: req.Offset,
		Count:  req.Count,
	})
	if err != nil {
		return nil, err
	}
	if res.ErrCode != 0 {
		return nil, errorx.WithCause(errorx.ErrNotFoundObject, res.ErrMsg)
	}

	return &types.GetOAMediaListReply{
		TotalCount: res.TotalCount,
		ItemCount:  res.ItemCount,
		Item:       res.Item,
	}, nil
}
