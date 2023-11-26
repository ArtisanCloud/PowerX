package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetOAMediaByVideoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOAMediaByVideoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOAMediaByVideoLogic {
	return &GetOAMediaByVideoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetOAMediaByVideoLogic) GetOAMediaByVideo(req *types.GetOAMediaRequest) (resp *types.GetOAMediaByVideoReply, err error) {
	res, err := l.svcCtx.PowerX.WechatOA.App.Material.GetVideo(l.ctx, req.MediaId)

	return &types.GetOAMediaByVideoReply{
		Title:       res.Title,
		Description: res.Description,
		DownUrl:     res.DownUrl,
	}, nil
}
