package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

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
	// todo: add your logic here and delete this line

	return
}
