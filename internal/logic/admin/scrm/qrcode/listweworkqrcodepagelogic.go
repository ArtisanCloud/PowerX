package qrcode

import (
	"PowerX/internal/model/scene"
	"context"
	"strings"
	"time"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkQrcodePageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListWeWorkQrcodePageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkQrcodePageLogic {
	return &ListWeWorkQrcodePageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// ListWeWorkQrcodePage
//  @Description: 群活码列表
//  @receiver qrcode
//  @param req
//  @return resp
//  @return err
//
func (qrcode *ListWeWorkQrcodePageLogic) ListWeWorkQrcodePage(opt *types.ListWeWorkGroupQrcodeActiveReqeust) (resp *types.ListWeWorkQrcodeActiveReply, err error) {

	reply, err := qrcode.svcCtx.PowerX.SCRM.Wechat.FindWeWorkCustomerGroupQrcodePage(qrcode.OPT(opt))

	return &types.ListWeWorkQrcodeActiveReply{
		List:      qrcode.DTO(reply),
		PageIndex: reply.PageIndex,
		PageSize:  reply.PageSize,
		Total:     reply.Total,
	}, err
}

//
//
//  @Description:
//  @receiver qrcode
//  @param opt
//  @return *types.PageOption[types.ListWeWorkGroupQrcodeActiveReqeust]
//
func (qrcode *ListWeWorkQrcodePageLogic) OPT(opt *types.ListWeWorkGroupQrcodeActiveReqeust) *types.PageOption[types.ListWeWorkGroupQrcodeActiveReqeust] {

	option := types.PageOption[types.ListWeWorkGroupQrcodeActiveReqeust]{
		Option:    types.ListWeWorkGroupQrcodeActiveReqeust{},
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
	}
	if v := opt.UserId; v != `` {
		option.Option.UserId = v
	}
	if v := opt.Name; v != `` {
		option.Option.Name = v
	}
	if v := opt.Qid; v != `` {
		option.Option.Qid = v
	}
	if v := opt.State; v > 0 {
		option.Option.State = v
	}
	return &option

}

//
// DTO
//  @Description:
//  @receiver qrcode
//  @param data
//  @return reply
//
func (qrcode *ListWeWorkQrcodePageLogic) DTO(data *types.Page[*scene.SceneQrcode]) (reply []*types.WeWorkQrcodeActive) {

	if data.List != nil {
		for _, obj := range data.List {
			reply = append(reply, qrcode.dto(obj))
		}
	}

	return reply
}

//
// dto
//  @Description:
//  @receiver qrcode
//  @param obj
//  @return *types.WeWorkQrcodeActive
//
func (qrcode *ListWeWorkQrcodePageLogic) dto(obj *scene.SceneQrcode) *types.WeWorkQrcodeActive {

	return &types.WeWorkQrcodeActive{
		QId:                obj.QId,
		Name:               obj.Name,
		Desc:               obj.Desc,
		Owner:              strings.Split(obj.Owner, `,`),
		RealQrcodeLink:     obj.RealQrcodeLink,
		Platform:           obj.Platform,
		Classify:           obj.Classify,
		SceneLink:          obj.SceneLink,
		SafeThresholdValue: obj.SafeThresholdValue,
		ExpiryDate:         obj.ExpiryDate,
		ExpiryState:        int(obj.ExpiryDate - time.Now().Unix()),
		ActiveQrcodeLink:   obj.ActiveQrcodeLink,
		CPA:                obj.Cpa,
		State:              obj.State,
	}
}
