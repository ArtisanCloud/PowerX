package tag

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"
	tagReq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/tag/request"
	"github.com/zeromicro/go-zero/core/logx"
)

type CreateWeWorkCropTagLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateWeWorkCropTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateWeWorkCropTagLogic {
	return &CreateWeWorkCropTagLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (tag *CreateWeWorkCropTagLogic) CreateWeWorkCropTag(opt *types.CreateCorpTagRequest) (resp *types.StatusWeWorkReply, err error) {

	cropTag, err := tag.OPT(opt)
	if err != nil {
		return nil, err
	}
	_, err = tag.svcCtx.PowerX.SCRM.Wechat.CreateWeWorkCorpTagRequest(cropTag)

	if err != nil {
		return nil, err
	}
	return &types.StatusWeWorkReply{
		Status: `success`,
	}, err

}

//
// (opt *types.CreateCorpTagRequest)
//  @Description:
//  @receiver tag
//  @param opt
//  @return cropTag
//  @return err
//
func (tag *CreateWeWorkCropTagLogic) OPT(opt *types.CreateCorpTagRequest) (cropTag *tagReq.RequestTagAddCorpTag, err error) {

	return &tagReq.RequestTagAddCorpTag{
		GroupID:   &opt.GroupId,
		GroupName: opt.GroupName,
		Order:     opt.Sort,
		Tag:       tag.loadTagField(opt.Tag),
		AgentID:   &opt.AgentId,
	}, err
}

//
// loadTagFeild
//  @Description:
//  @receiver tag
//  @param tags
//  @return obj
//
func (tag *CreateWeWorkCropTagLogic) loadTagField(tags []*types.TagFieldTag) (obj []tagReq.RequestTagAddCorpTagFieldTag) {
	for _, val := range tags {
		obj = append(obj, tagReq.RequestTagAddCorpTagFieldTag{
			Name:  val.Name,
			Order: val.Sort,
		})
	}
	return obj
}
