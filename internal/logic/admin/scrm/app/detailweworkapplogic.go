package app

import (
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/agent/response"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type DetailWeWorkAppLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewDetailWeWorkAppLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DetailWeWorkAppLogic {
    return &DetailWeWorkAppLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// DetailWeWorkApp
//  @Description: 应用详情
//  @receiver this
//  @param req
//  @return resp
//  @return err
//
func (app *DetailWeWorkAppLogic) DetailWeWorkApp(opt *types.ApplicationRequest) (resp *types.ApplicationReply, err error) {

    reply, err := app.svcCtx.PowerX.SCRM.Wechat.PullDetailWeWorkAppRequest(opt.AgentId)

    return app.DTO(reply), err
}

//
// DTO
//  @Description:
//  @receiver this
//  @param detail
//  @return *types.ApplicationReply
//
func (app *DetailWeWorkAppLogic) DTO(detail *response.ResponseAgentGet) *types.ApplicationReply {

    return &types.ApplicationReply{
        Agentid:                 detail.AgentID,
        Name:                    detail.Name,
        SquareLogoUrl:           detail.SquareLogoURL,
        Description:             detail.Description,
        AllowUserinfos:          app.allowUserInfos(detail.AllowUserInfos),
        AllowPartys:             app.allowPartys(detail.AllowParty),
        AllowTags:               app.allowTags(detail.AllowTags),
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
func (app *DetailWeWorkAppLogic) allowUserInfos(infos response.ResponseAgentAllowUserInfos) (infox types.AllowUserinfos) {

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
func (app *DetailWeWorkAppLogic) allowPartys(party response.ResponseAgentAllowParty) types.AllowPartys {
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
func (app *DetailWeWorkAppLogic) allowTags(tags response.ResponseAgentAllowTags) types.AllowTags {
    return types.AllowTags{
        Tagid: tags.TagID,
    }
}
