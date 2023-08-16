package mediaresource

import (
	"PowerX/internal/model/media"
	"PowerX/internal/types/errorx"
	"context"
	"net/http"
	"strconv"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

const MaxFileSize = 2 << 20

type CreateMediaResourceLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateMediaResourceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateMediaResourceLogic {
	return &CreateMediaResourceLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateMediaResourceLogic) CreateMediaResource(r *http.Request) (resp *types.CreateMediaResourceReply, err error) {

	queryParams := r.URL.Query()

	// 获取特定参数的值
	strSortIndex := queryParams.Get("sortIndex")
	//fmt.Dump(strSortIndex)
	sortIndex, err := strconv.Atoi(strSortIndex)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	err = r.ParseMultipartForm(MaxFileSize)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	file, handler, err := r.FormFile("resource")
	//fmt.Dump(handler.Filename)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}
	defer file.Close()

	resource, err := l.svcCtx.PowerX.MediaResource.MakeProductMediaResource(l.ctx, handler)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	resourceReply := TransformMediaResourceToReply(resource)
	if resourceReply != nil {
		resourceReply.SortIndex = sortIndex
	}
	return &types.CreateMediaResourceReply{
		MediaResource: resourceReply,
		IsOSS:         !resource.IsLocalStored,
	}, nil
}

func TransformMediaResourceToReply(resource *media.MediaResource) *types.MediaResource {
	if resource == nil {
		return nil
	}
	return &types.MediaResource{
		Id:            resource.Id,
		CustomerId:    resource.CustomerId,
		BucketName:    resource.BucketName,
		Filename:      resource.Filename,
		Size:          resource.Size,
		IsLocalStored: resource.IsLocalStored,
		Url:           resource.Url,
		ContentType:   resource.ContentType,
		ResourceType:  resource.ResourceType,
	}
}
