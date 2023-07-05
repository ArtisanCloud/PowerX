package app

import (
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/scrm/wechat"
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/power"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SendWeWorkAppGroupArticleMessageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSendWeWorkAppGroupArticleMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendWeWorkAppGroupArticleMessageLogic {
	return &SendWeWorkAppGroupArticleMessageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// SendWeWorkAppGroupArticleMessage
//  @Description:
//  @receiver this
//  @param req
//  @return resp
//  @return err
//
func (this *SendWeWorkAppGroupArticleMessageLogic) SendWeWorkAppGroupArticleMessage(opt *types.AppGroupMessageArticleRequest) (resp *types.AppGroupMessageReply, err error) {

	option, err := this.DTO(opt)
	if err != nil {
		return nil, errorx.ErrBadRequest
	}
	_, err = this.svcCtx.PowerX.SCRM.Wechat.PushAppWeWorkGroupMessageArticlesRequest(option, opt.SendTime)

	return &types.AppGroupMessageReply{
		ChatID: opt.ChatID,
	}, err

}

//
// DTO
//  @Description:
//  @receiver this
//  @param opt
//  @return *power.HashMap
//  @return error
//
func (this *SendWeWorkAppGroupArticleMessageLogic) DTO(opt *types.AppGroupMessageArticleRequest) (*power.HashMap, error) {
	option := wechat.WechatAppRequestBase{
		ChatID:  opt.ChatID,
		MsgType: `news`,
		Safe:    0,
		News:    wechat.WechatAppRequestNews{},
	}
	arc := wechat.WechatAppRequestNews{}
	arc.Article = append(arc.Article, &wechat.WechatAppRequestNewsArticle{
		Title:       opt.Title,
		Description: opt.Description,
		URL:         opt.URL,
		PicURL:      opt.PicURL,
	})
	option.News = arc
	hashMap, err := power.StructToHashMap(option)

	return hashMap,
		err
}
