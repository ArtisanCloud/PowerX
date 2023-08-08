package mediaresource

import (
	"PowerX/internal/model/media"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListMediaResourcesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListMediaResourcesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMediaResourcesLogic {
	return &ListMediaResourcesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListMediaResourcesLogic) ListMediaResources(req *types.ListMediaResourcesPageRequest) (resp *types.ListMediaResourcesPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}

func TransformResourceMediasToReplyForMP(pivots []*media.PivotMediaResourceToObject) (imagesReply []*types.MediaResource) {

	imagesReply = []*types.MediaResource{}
	for _, pivot := range pivots {
		imageReply := TransformMediaResourceToReplyForMP(pivot.MediaResource)
		imagesReply = append(imagesReply, imageReply)
	}
	return imagesReply
}

func TransformMediaResourceToReplyForMP(resource *media.MediaResource) *types.MediaResource {
	return &types.MediaResource{
		Id:            resource.Id,
		IsLocalStored: resource.IsLocalStored,
		Url:           resource.Url,
		Filename:      resource.Filename,
		ContentType:   resource.ContentType,
		ResourceType:  resource.ResourceType,
	}
}
