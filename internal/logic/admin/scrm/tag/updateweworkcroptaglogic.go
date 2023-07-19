package tag

import (
	"context"
	tagReq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/tag/request"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateWeWorkCropTagLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateWeWorkCropTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateWeWorkCropTagLogic {
	return &UpdateWeWorkCropTagLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// UpdateWeWorkCropTag
//  @Description:
//  @receiver tag
//  @param opt
//  @return resp
//  @return err
//
func (tag *UpdateWeWorkCropTagLogic) UpdateWeWorkCropTag(opt *types.UpdateCorpTagRequest) (resp *types.StatusWeWorkReply, err error) {

	cropTag, err := tag.OPT(opt)
	if err != nil {
		return nil, err
	}
	_, err = tag.svcCtx.PowerX.SCRM.Wechat.UpdateWeWorkCorpTagRequest(cropTag)

	if err != nil {
		return nil, err
	}
	return &types.StatusWeWorkReply{
		Status: `success`,
	}, err
}

//
// (opt *types.UpdateCorpTagRequest)
//  @Description:
//  @receiver tag
//  @param opt
//  @return cropTag
//  @return err
//
func (tag *UpdateWeWorkCropTagLogic) OPT(opt *types.UpdateCorpTagRequest) (cropTag *tagReq.RequestTagEditCorpTag, err error) {

	return &tagReq.RequestTagEditCorpTag{
		ID:      opt.TagId,
		Name:    opt.Name,
		Order:   opt.Sort,
		AgentID: &opt.AgentId,
	}, err
}
