package bot

import (
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/groupRobot/request"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type WechatBotArticlesLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewWechatBotArticlesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WechatBotArticlesLogic {
    return &WechatBotArticlesLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// WechatBotArticles
//  @Description:
//  @receiver this
//  @param req
//  @return resp
//  @return err
//
func (this *WechatBotArticlesLogic) WechatBotArticles(req *types.GroupRobotMsgNewsArticlesRequest) (resp *types.GroupRobotMsgNewsArticlesReplay, err error) {

    articles := []*request.GroupRobotMsgNewsArticles{
        {Title: req.Title, Description: req.Description, Url: req.Url, PicUrl: req.PicUrl},
    }
    replay, err := this.svcCtx.PowerX.SCRM.Wechat.BotArticles(req.Key, articles)
    resp.Messaage = replay.Message

    return resp, err
}
