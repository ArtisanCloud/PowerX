package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMediaResourceLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetMediaResourceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMediaResourceLogic {
	return &GetMediaResourceLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetMediaResourceLogic) GetMediaResource(req *types.GetMediaResourceRequest) (resp *types.GetMediaResourceReply, err error) {
	// todo: add your logic here and delete this line

	return
}
