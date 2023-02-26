package contact

import (
	"PowerX/internal/uc"
	"context"
	"fmt"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateLiveQRCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateLiveQRCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateLiveQRCodeLogic {
	return &CreateLiveQRCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateLiveQRCodeLogic) CreateLiveQRCode(req *types.CreateLiveQRCodeRequest) (resp *types.CreateLiveQRCodeReply, err error) {
	var code uc.LiveQRCode
	switch req.Type {
	case "WEB":
		code.RedirectTo = req.Web.Url
		code.IconUrl = req.IconUrl
		l.svcCtx.UC.Contact.CreateLiveQRCode(l.ctx, &code)
	}

	resp = &types.CreateLiveQRCodeReply{
		Uri: fmt.Sprintf("/public/v1/live-qr-code/%s", code.Uid),
	}
	return
}
