package resource

import (
    "PowerX/internal/svc"
    "PowerX/internal/types"
    "PowerX/internal/types/errorx"
    "context"
    "net/http"
    "os"

    "github.com/zeromicro/go-zero/core/logx"
)

type CreateWeWorkImageLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewCreateWeWorkImageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateWeWorkImageLogic {
    return &CreateWeWorkImageLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// CreateWeWorkImage
//  @Description:
//  @receiver image
//  @param r
//  @return resp
//  @return err
//
func (image *CreateWeWorkImageLogic) CreateWeWorkImage(r *http.Request) (resp *types.CreateWeWorkSourceImageReply, err error) {

    err = r.ParseMultipartForm(2 << 20)
    if err != nil {
        return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
    }
    var uri string
    file, handler, err := r.FormFile("resource")

    if err != nil || handler == nil {
        uri = r.FormValue("link")
        uri = `.` + uri
        if _, err := os.Stat(uri); os.IsNotExist(err) {
            return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
        }
    }

    link, err := image.svcCtx.PowerX.SCRM.Wechat.UploadImageToResourceRequest(uri, handler)

    if err != nil {
        return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
    }
    if file != nil {
        file.Close()
    }

    return &types.CreateWeWorkSourceImageReply{
        Link: link,
    }, err

}
