package app

import (
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/agent/response"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type WechatAppListLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewWechatAppListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WechatAppListLogic {
    return &WechatAppListLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// WechatAppList
//  @Description: App列表
//  @receiver this
//  @return resp
//  @return err
//
func (this *WechatAppListLogic) WechatAppList() (resp *types.AppWechatListReply, err error) {

    reply, err := this.svcCtx.PowerX.SCRM.Wechat.AppWechatListRequest()

    return &types.AppWechatListReply{
        List: this.DTO(reply),
    }, err
}

//
// DTO
//  @Description:
//  @receiver this
//  @param list
//  @return apps
//
func (this *WechatAppListLogic) DTO(list *response.ResponseAgentList) (apps []*types.AppWechat) {

    for _, obj := range list.AgentList {
        apps = append(apps, &types.AppWechat{
            Agentid:       obj.AgentID,
            Name:          obj.Name,
            SquareLogoUrl: obj.SquareLogoURL,
        })
    }

    return apps

}
