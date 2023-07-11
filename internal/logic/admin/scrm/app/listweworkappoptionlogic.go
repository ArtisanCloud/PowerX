package app

import (
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/agent/response"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkAppOptionLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewListWeWorkAppOptionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkAppOptionLogic {
    return &ListWeWorkAppOptionLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// ListWeWorkAppOption
//  @Description:	应用列表
//  @receiver this
//  @return resp
//  @return err
//
func (app *ListWeWorkAppOptionLogic) ListWeWorkAppOption() (resp *types.AppWeWorkListReply, err error) {

    reply, err := app.svcCtx.PowerX.SCRM.Wechat.PullListWeWorkAppRequest()

    return &types.AppWeWorkListReply{
        List: app.DTO(reply),
    }, err
}

//
// DTO
//  @Description:
//  @receiver this
//  @param list
//  @return apps
//
func (app *ListWeWorkAppOptionLogic) DTO(list *response.ResponseAgentList) (apps []*types.AppWechat) {

    for _, obj := range list.AgentList {
        apps = append(apps, &types.AppWechat{
            Agentid:       obj.AgentID,
            Name:          obj.Name,
            SquareLogoUrl: obj.SquareLogoURL,
        })
    }

    return apps

}
