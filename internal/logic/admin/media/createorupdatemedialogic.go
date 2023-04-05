package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateOrUpdateMediaLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateOrUpdateMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateOrUpdateMediaLogic {
	return &CreateOrUpdateMediaLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateOrUpdateMediaLogic) CreateOrUpdateMedia(req *types.CreateOrUpdateMediaRequest) (resp *types.CreateOrUpdateMediaReply, err error) {
	// todo: add your logic here and delete this line

	return
}
