package media

import (
	fmt "PowerX/pkg/printx"
	"context"
	"net/http"

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
	err = r.ParseMultipartForm(MaxFileSize)
	if err != nil {
		return nil, err
	}

	file, handler, err := r.FormFile("resource")
	//fmt.Dump(handler.Filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	resource, err := l.svcCtx.PowerX.MediaResource.CreateProductResource(l.ctx, handler)
	if err != nil {
		return nil, err
	}
	fmt.Dump(resource)

	return
}
