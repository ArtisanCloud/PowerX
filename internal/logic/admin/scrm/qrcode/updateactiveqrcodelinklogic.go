package qrcode

import (
	"PowerX/internal/types/errorx"
	"context"
	"fmt"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateActiveQrcodeLinkLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateActiveQrcodeLinkLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateActiveQrcodeLinkLogic {
	return &UpdateActiveQrcodeLinkLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateActiveQrcodeLink
//
//	@Description:
//	@receiver qrcode
//	@param opt
//	@return resp
//	@return err
func (qrcode *UpdateActiveQrcodeLinkLogic) UpdateActiveQrcodeLink(opt *types.ActionRequest) (resp *types.ActionWeWorkGroupQrcodeActiveReply, err error) {

	err = qrcode.OPT(opt)
	if err != nil {
		return nil, err
	}
	err = qrcode.svcCtx.PowerX.SCRM.Wechat.UpdateSceneQRCodeLink(opt.Qid, opt.SceneQRCodeLink)
	if err != nil {
		return nil, errorx.ErrBadRequest
	}

	return &types.ActionWeWorkGroupQrcodeActiveReply{
		Status: `success`,
	}, err
}

// OPT
//
//	@Description:
//	@receiver qrcode
//	@param opt
//	@return error
func (qrcode *UpdateActiveQrcodeLinkLogic) OPT(opt *types.ActionRequest) error {

	if opt.Qid == `` {
		return fmt.Errorf(`Qid error`)
	}
	if opt.SceneQRCodeLink == `` {
		return fmt.Errorf(`SceneQRCodeLink error`)
	}
	return nil

}
