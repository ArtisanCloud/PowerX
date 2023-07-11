package wechat

import (
    "PowerX/internal/model/powermodel"
    "PowerX/internal/model/scrm/resource"
    "PowerX/internal/types"
    "bytes"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/power"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/media/response"
    "mime/multipart"
    "strings"
)

//
// UploadImageToResourceRequest
//  @Description:
//  @receiver this
//  @param path
//  @return link
//  @return err
//
func (this *wechatUseCase) UploadImageToResourceRequest(localPath string, handle *multipart.FileHeader) (link string, err error) {

    var reply *response.ResponseUploadImage
    var fileName string
    var fileSize int
    if localPath != `` {
        reply, fileName, err = this.uploadLocalUriToWeWorkLink(localPath)
    } else {
        reply, err = this.uploadLocalFileToWeWorkLink(handle)
        fileName = handle.Filename
        fileSize = int(handle.Size)
    }
    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.wework.upload.image.error.`, reply.ResponseWork)
    }

    if err == nil {
        this.modelWeworkResource.resource.Action(this.db, []*resource.WeWorkResource{
            {
                Url:          reply.URL,
                FileName:     fileName,
                Remark:       ``,
                BucketName:   ``,
                Size:         fileSize,
                ResourceType: `image`,
            },
        })
    }

    return reply.URL, err

}

//
// uploadLocalFileToWeWorkLink
//  @Description:
//  @receiver this
//  @param handle
//  @return *response.ResponseUploadImage
//  @return error
//
func (this *wechatUseCase) uploadLocalFileToWeWorkLink(handle *multipart.FileHeader) (*response.ResponseUploadImage, error) {

    hms := power.HashMap{}
    bts := bytes.Buffer{}
    hms[`name`] = handle.Filename
    hms[`value`] = func(han *multipart.FileHeader) []byte {
        open, _ := han.Open()
        n, _ := bts.ReadFrom(open)
        if n > 0 {
            return bts.Bytes()
        }
        return nil
    }(handle)
    reply, err := this.wework.Media.UploadImage(this.ctx, ``, &hms)

    return reply, err
}

//
// uploadLocalUriToWeWorkLink
//  @Description:
//  @receiver this
//  @param localPath
//  @return *response.ResponseUploadImage
//  @return string
//  @return error
//
func (this *wechatUseCase) uploadLocalUriToWeWorkLink(localPath string) (*response.ResponseUploadImage, string, error) {

    split := strings.Split(localPath, `/`)
    name := split[len(split)-1 : len(split)]
    reply, err := this.wework.Media.UploadImage(this.ctx, localPath, nil)

    return reply, name[0], err
}

//
// FindWeWorkResourceListFromLocalPage
//  @Description:
//  @receiver this
//  @param opt
//  @return *types.Page[*resource.WeWorkResource]
//  @return error
//
func (this *wechatUseCase) FindWeWorkResourceListFromLocalPage(opt *types.ListWeWorkResourceImageRequest) (*types.Page[*resource.WeWorkResource], error) {

    var resources []*resource.WeWorkResource
    var count int64

    query := this.db.WithContext(this.ctx).Table(this.modelWeworkResource.resource.TableName())

    if opt.PageIndex == 0 {
        opt.PageIndex = 1
    }
    if opt.PageSize == 0 {
        opt.PageSize = powermodel.PageDefaultSize
    }
    if v := opt.ResourceType; v != `` {
        query.Where(`resource_type = ?`, v)
    }
    if err := query.Count(&count).Error; err != nil {
        return nil, err
    }
    if opt.PageIndex != 0 && opt.PageSize != 0 {
        query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
    }

    err := query.Find(&resources).Error

    return &types.Page[*resource.WeWorkResource]{
        List:      resources,
        PageIndex: opt.PageIndex,
        PageSize:  opt.PageSize,
        Total:     count,
    }, err
}
