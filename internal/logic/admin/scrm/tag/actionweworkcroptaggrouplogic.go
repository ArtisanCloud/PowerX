package tag

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ActionWeWorkCropTagGroupLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewActionWeWorkCropTagGroupLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ActionWeWorkCropTagGroupLogic {
	return &ActionWeWorkCropTagGroupLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// ActionWeWorkCropTagGroup
//  @Description:
//  @receiver group
//  @param opt
//  @return resp
//  @return err
//
func (group *ActionWeWorkCropTagGroupLogic) ActionWeWorkCropTagGroup(opt *types.ActionCorpTagGroupRequest) (resp *types.StatusWeWorkReply, err error) {

	/*if len(opt.Tags) == 0 {
		return nil, errorx.ErrBadRequest
	}*/

	_, err = group.svcCtx.PowerX.SCRM.Wechat.ActionWeWorkCorpTagGroupRequest(opt)
	if err != nil {
		return nil, err
	}

	return &types.StatusWeWorkReply{
		Status: `success`,
	}, err

}
