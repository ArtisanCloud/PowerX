package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMediaListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetMediaListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMediaListLogic {
	return &GetMediaListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetMediaListLogic) GetMediaList(req *types.GetMediaListRequest) (resp *types.GetMediaListReply, err error) {
	// todo: add your logic here and delete this line

	return
}
