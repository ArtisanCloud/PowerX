package media

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteOAMediaLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteOAMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteOAMediaLogic {
	return &DeleteOAMediaLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteOAMediaLogic) DeleteOAMedia(req *types.DeleteOAMediaRequest) (resp *types.DeleteOAMediaReply, err error) {
	res, err := l.svcCtx.PowerX.WechatOA.App.Material.Delete(l.ctx, req.MediaId)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	if res.ErrCode != 0 {
		return nil, errorx.WithCause(errorx.ErrBadRequest, res.ErrMsg)
	}

	return &types.DeleteOAMediaReply{
		Success: true,
		Data:    nil,
	}, nil
}
