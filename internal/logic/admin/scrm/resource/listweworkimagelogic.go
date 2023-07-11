package resource

import (
    "PowerX/internal/model/scrm/resource"
    "context"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkImageLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewListWeWorkImageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkImageLogic {
    return &ListWeWorkImageLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// ListWeWorkImage
//  @Description:
//  @receiver image
//  @param req
//  @return resp
//  @return err
//
func (image *ListWeWorkImageLogic) ListWeWorkImage(opt *types.ListWeWorkResourceImageRequest) (resp *types.ListWeWorkResourceImageReply, err error) {

    page, err := image.svcCtx.PowerX.SCRM.Wechat.FindWeWorkResourceListFromLocalPage(opt)

    return &types.ListWeWorkResourceImageReply{
        List:      image.DTO(page.List),
        PageIndex: page.PageIndex,
        PageSize:  page.PageSize,
        Total:     page.Total,
    }, err

}

//
// DTO
//  @Description:
//  @receiver image
//  @param data
//  @return resources
//
func (image *ListWeWorkImageLogic) DTO(data []*resource.WeWorkResource) (resources []*types.Resource) {

    if data != nil {
        for _, obj := range data {
            resources = append(resources, &types.Resource{
                Link:         obj.Url,
                ResourceType: obj.ResourceType,
                CreateTime:   obj.CreatedAt.String(),
            })
        }
    }
    return resources

}
