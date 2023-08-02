package bot

import (
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/groupRobot/request"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type BotWeWorkArticlesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotWeWorkArticlesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotWeWorkArticlesLogic {
	return &BotWeWorkArticlesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// BotWeWorkArticles
//
//	@Description: 机器人发送图文
//	@receiver bot
//	@param req
//	@return resp
//	@return err
func (bot *BotWeWorkArticlesLogic) BotWeWorkArticles(req *types.GroupRobotMsgNewsArticlesRequest) (resp *types.GroupRobotMsgNewsArticlesReply, err error) {

	articles := []*request.GroupRobotMsgNewsArticles{
		{Title: req.Title, Description: req.Description, Url: req.Url, PicUrl: req.PicUrl},
	}
	replay, err := bot.svcCtx.PowerX.SCRM.Wechat.PushWeWorkBotArticlesRequest(req.Key, articles)
	resp.Messaage = replay.Message

	return resp, err
}
