package public

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AccessLiveQRCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAccessLiveQRCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AccessLiveQRCodeLogic {
	return &AccessLiveQRCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AccessLiveQRCodeLogic) AccessLiveQRCode(req *types.AccessLiveQRCodeRequest) (resp *types.AccessLiveQRCodeReply, err error) {
	code := l.svcCtx.UC.Contact.GetLiveQRCode(l.ctx, req.Uid)
	resp = &types.AccessLiveQRCodeReply{
		RedirectTo: code.RedirectTo,
	}
	return
}
