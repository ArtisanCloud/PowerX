package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateMediaUploadRequestLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateMediaUploadRequestLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateMediaUploadRequestLogic {
	return &CreateMediaUploadRequestLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateMediaUploadRequestLogic) CreateMediaUploadRequest(req *types.CreateMediaUploadRequest) (resp *types.CreateMediaUploadRequestReply, err error) {
	// todo: add your logic here and delete this line

	return
}
