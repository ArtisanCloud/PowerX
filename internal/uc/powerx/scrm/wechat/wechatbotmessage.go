package wechat

import (
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/groupRobot/request"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/groupRobot/response"
    "github.com/zeromicro/go-zero/core/logx"
)

//
// PushWeWorkBotTextRequest
//  @Description:
//  @receiver this
//  @param key
//  @param text
//  @return error
//
func (this wechatUseCase) PushWeWorkBotTextRequest(key string, text *request.GroupRobotMsgText) error {

    reply, err := this.wework.GroupRobot.SendText(context.TODO(), key, text)
    logx.Debug(reply, err)
    return err

}

//
// PushWeWorkBotFileRequest
//  @Description:
//  @receiver this
//  @param key
//  @param file
//  @return error
//
func (this wechatUseCase) PushWeWorkBotFileRequest(key string, file *request.GroupRobotMsgFile) error {

    reply, err := this.wework.GroupRobotMessenger.SendFile(context.TODO(), key, file)
    logx.Debug(reply, err)
    return err

}

//
// PushWeWorkBotImageRequest
//  @Description:
//  @receiver this
//  @param key
//  @param file
//  @return error
//
func (this wechatUseCase) PushWeWorkBotImageRequest(key string, image *request.GroupRobotMsgImage) error {

    reply, err := this.wework.GroupRobotMessenger.SendImage(context.TODO(), key, image)
    logx.Debug(reply, err)
    return err

}

//
// PushWeWorkBotMarkdownRequest
//  @Description:
//  @receiver this
//  @param key
//  @param markdown
//  @return error
//
func (this wechatUseCase) PushWeWorkBotMarkdownRequest(key string, markdown *request.GroupRobotMsgMarkdown) error {

    reply, err := this.wework.GroupRobotMessenger.SendMarkdown(context.TODO(), key, markdown)
    logx.Debug(reply, err)
    return err

}

//
// PushWeWorkBotArticlesRequest
//  @Description:
//  @receiver this
//  @param key
//  @param articles
//  @return *response.ResponseGroupRobotSend
//  @return error
//
func (this wechatUseCase) PushWeWorkBotArticlesRequest(key string, articles []*request.GroupRobotMsgNewsArticles) (resp *response.ResponseGroupRobotSend, err error) {

    reply, err := this.wework.GroupRobotMessenger.SendNewsArticles(this.ctx, key, articles)
    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.push.wework.bot.articles.error`, reply.ResponseWork)
    }
    return reply, err

}

//
// PushWeWorkBotTemplateRequest
//  @Description:
//  @receiver this
//  @param key
//  @param template
//  @return error
//
func (this wechatUseCase) PushWeWorkBotTemplateRequest(key string, template *request.GroupRobotMsgTemplateCard) error {

    reply, err := this.wework.GroupRobotMessenger.SendTemplateCard(context.TODO(), key, template)
    logx.Debug(reply, err)
    return err

}
