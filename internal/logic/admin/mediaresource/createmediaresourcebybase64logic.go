package mediaresource

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateMediaResourceByBase64Logic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateMediaResourceByBase64Logic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateMediaResourceByBase64Logic {
	return &CreateMediaResourceByBase64Logic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateMediaResourceByBase64Logic) CreateMediaResourceByBase64(req *types.CreateMediaResourceByBase64Request) (resp *types.CreateMediaResourceReply, err error) {

	resource, err := l.svcCtx.PowerX.MediaResource.MakeOSSResourceByBase64(l.ctx, req.BucketName, req.Base64Data)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	l.svcCtx.PowerX.MediaResource.CreateMediaResource(l.ctx, resource)

	return &types.CreateMediaResourceReply{
		MediaResource: TransformMediaResourceToResourceReply(resource),
		IsOSS:         !resource.IsLocalStored,
	}, nil
}
