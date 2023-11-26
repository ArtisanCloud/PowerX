package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMediaByVideoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetMediaByVideoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMediaByVideoLogic {
	return &GetMediaByVideoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetMediaByVideoLogic) GetMediaByVideo(req *types.GetMediaRequest) (resp *types.GetMediaReply, err error) {
	// todo: add your logic here and delete this line

	return
}
