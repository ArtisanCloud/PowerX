package app

import (
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/agent/response"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type WechatAppDetailLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewWechatAppDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WechatAppDetailLogic {
	return &WechatAppDetailLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// WechatAppDetail
//  @Description:	App详情
//  @receiver this
//  @param opt
//  @return resp
//  @return err
//
func (this *WechatAppDetailLogic) WechatAppDetail(opt *types.ApplicationRequest) (resp *types.ApplicationReply, err error) {

	reply, err := this.svcCtx.PowerX.SCRM.Wechat.AppWechatDetailRequest(opt.AgentID)

	return this.DTO(reply), err

}

//
// DTO
//  @Description:
//  @receiver this
//  @param detail
//  @return *types.ApplicationReply
//
func (this *WechatAppDetailLogic) DTO(detail *response.ResponseAgentGet) *types.ApplicationReply {

	return &types.ApplicationReply{
		Agentid:                 detail.AgentID,
		Name:                    detail.Name,
		SquareLogoUrl:           detail.SquareLogoURL,
		Description:             detail.Description,
		AllowUserinfos:          this.allowUserInfos(detail.AllowUserInfos),
		AllowPartys:             this.allowPartys(detail.AllowParty),
		AllowTags:               this.allowTags(detail.AllowTags),
		Close:                   int(detail.Close),
		RedirectDomain:          detail.RedirectDomain,
		ReportLocationFlag:      int(detail.ReportLocationFlag),
		Isreportenter:           int(detail.IsReportEnter),
		HomeUrl:                 detail.HomeURL,
		CustomizedPublishStatus: 0,
	}

}

//
// allowUserInfos
//  @Description:
//  @receiver this
//
func (this *WechatAppDetailLogic) allowUserInfos(infos response.ResponseAgentAllowUserInfos) (infox types.AllowUserinfos) {

	for _, user := range infos.User {
		infox.User = append(infox.User, types.User{
			Userid: user.UserID,
		})
	}
	return infox

}

//
// allowPartys
//  @Description:
//  @receiver this
//  @param party
//  @return types.AllowPartys
//
func (this *WechatAppDetailLogic) allowPartys(party response.ResponseAgentAllowParty) types.AllowPartys {
	return types.AllowPartys{
		Partyid: party.PartyID,
	}
}

//
// allowTags
//  @Description:
//  @receiver this
//  @param tags
//  @return types.AllowTags
//
func (this *WechatAppDetailLogic) allowTags(tags response.ResponseAgentAllowTags) types.AllowTags {
	return types.AllowTags{
		Tagid: tags.TagID,
	}
}
