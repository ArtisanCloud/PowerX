package tag

import (
	"PowerX/internal/types/errorx"
	"context"
	tagReq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/tag/request"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ActionWeWorkCustomerTagLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewActionWeWorkCustomerTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ActionWeWorkCustomerTagLogic {
	return &ActionWeWorkCustomerTagLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// ActionWeWorkCustomerTag
//  @Description:
//  @receiver customer
//  @param opt
//  @return resp
//  @return err
//
func (customer *ActionWeWorkCustomerTagLogic) ActionWeWorkCustomerTag(opt *types.ActionCustomerTagRequest) (resp *types.StatusWeWorkReply, err error) {

	option, err := customer.OPT(opt)
	if err != nil {
		return nil, err
	}
	_, err = customer.svcCtx.PowerX.SCRM.Wechat.ActionWeWorkCustomerTagRequest(option)

	return &types.StatusWeWorkReply{
		Status: `success`,
	}, err

}

//
// OPT
//  @Description:
//  @receiver customer
//  @param opt
//  @return option
//  @return err
//
func (customer *ActionWeWorkCustomerTagLogic) OPT(opt *types.ActionCustomerTagRequest) (option *tagReq.RequestTagMarkTag, err error) {

	if opt.AddTag == nil && opt.RemoveTag == nil {
		return option, errorx.ErrBadRequest
	}
	return &tagReq.RequestTagMarkTag{
		UserID:         opt.UserId,
		ExternalUserID: opt.ExternalUserId,
		AddTag:         opt.AddTag,
		RemoveTag:      opt.RemoveTag,
	}, err
}
