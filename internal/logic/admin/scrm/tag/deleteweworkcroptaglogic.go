package tag

import (
	"PowerX/internal/types/errorx"
	"context"
	tagReq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/tag/request"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteWeWorkCropTagLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteWeWorkCropTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteWeWorkCropTagLogic {
	return &DeleteWeWorkCropTagLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// DeleteWeWorkCropTag
//  @Description:
//  @receiver tag
//  @param opt
//  @return resp
//  @return err
//
func (tag *DeleteWeWorkCropTagLogic) DeleteWeWorkCropTag(opt *types.DeleteCorpTagRequest) (resp *types.StatusWeWorkReply, err error) {

	option, err := tag.OPT(opt)
	if err != nil {
		return nil, err
	}
	_, err = tag.svcCtx.PowerX.SCRM.Wechat.DeleteWeWorkCorpTagRequest(option)
	if err != nil {
		return nil, err
	}
	return &types.StatusWeWorkReply{
		Status: `success`,
	}, err

}

//
// OPT
//  @Description:
//  @receiver tag
//  @param opt
//  @return *tagReq.RequestTagDelCorpTag
//  @return error
//
func (tag *DeleteWeWorkCropTagLogic) OPT(opt *types.DeleteCorpTagRequest) (*tagReq.RequestTagDelCorpTag, error) {
	if opt == nil {
		return nil, errorx.ErrBadRequest
	}
	return &tagReq.RequestTagDelCorpTag{
		TagID:   opt.TagIds,
		GroupID: opt.GroupIds,
		AgentID: &opt.AgentId,
	}, nil
}
