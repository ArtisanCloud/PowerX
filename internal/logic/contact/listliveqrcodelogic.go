package contact

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListLiveQRCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListLiveQRCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListLiveQRCodeLogic {
	return &ListLiveQRCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListLiveQRCodeLogic) ListLiveQRCode(req *types.ListLiveQRCodeRequest) (resp *types.ListLiveQRCodeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
