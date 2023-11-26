package media

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"
	"io"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetOAMediaLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOAMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOAMediaLogic {
	return &GetOAMediaLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetOAMediaLogic) GetOAMedia(req *types.GetOAMediaRequest) (resp *types.GetOAMediaReply, err error) {

	res, err := l.svcCtx.PowerX.WechatOA.App.Material.Get(l.ctx, req.MediaId)
	defer res.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return &types.GetOAMediaReply{
		OAMedia: body,
	}, nil
}
