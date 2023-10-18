package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateOAMediaLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateOAMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateOAMediaLogic {
	return &CreateOAMediaLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateOAMediaLogic) CreateOAMedia(req *types.CreateOAMediaRequest) (resp *types.CreateOAMediaReply, err error) {
	// todo: add your logic here and delete this line

	return
}
