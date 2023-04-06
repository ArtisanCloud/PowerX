package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMediaByKeyLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetMediaByKeyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMediaByKeyLogic {
	return &GetMediaByKeyLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetMediaByKeyLogic) GetMediaByKey(req *types.GetMediaByKeyRequest) (resp *types.GetMediaByKeyReply, err error) {
	// todo: add your logic here and delete this line

	return
}
