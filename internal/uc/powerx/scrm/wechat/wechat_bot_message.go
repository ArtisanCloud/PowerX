package wechat

import (
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/groupRobot/request"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/groupRobot/response"
    "github.com/zeromicro/go-zero/core/logx"
)

//
// BotText
//  @Description:
//  @receiver this
//  @param key
//  @param text
//  @return error
//
func (this wechatUseCase) BotText(key string, text *request.GroupRobotMsgText) error {

    reply, err := this.wework.GroupRobot.SendText(context.TODO(), key, text)
    logx.Debug(reply, err)
    return err

}

//
// BotFile
//  @Description:
//  @receiver this
//  @param key
//  @param file
//  @return error
//
func (this wechatUseCase) BotFile(key string, file *request.GroupRobotMsgFile) error {

    reply, err := this.wework.GroupRobotMessenger.SendFile(context.TODO(), key, file)
    logx.Debug(reply, err)
    return err

}

//
// BotImage
//  @Description:
//  @receiver this
//  @param key
//  @param file
//  @return error
//
func (this wechatUseCase) BotImage(key string, image *request.GroupRobotMsgImage) error {

    reply, err := this.wework.GroupRobotMessenger.SendImage(context.TODO(), key, image)
    logx.Debug(reply, err)
    return err

}

//
// BotMarkdown
//  @Description:
//  @receiver this
//  @param key
//  @param markdown
//  @return error
//
func (this wechatUseCase) BotMarkdown(key string, markdown *request.GroupRobotMsgMarkdown) error {

    reply, err := this.wework.GroupRobotMessenger.SendMarkdown(context.TODO(), key, markdown)
    logx.Debug(reply, err)
    return err

}

//
// BotArticles
//  @Description:
//  @receiver this
//  @param key
//  @param articles
//  @return *response.ResponseGroupRobotSend
//  @return error
//
func (this wechatUseCase) BotArticles(key string, articles []*request.GroupRobotMsgNewsArticles) (resp *response.ResponseGroupRobotSend, err error) {
    reply, err := this.wework.GroupRobotMessenger.SendNewsArticles(context.TODO(), key, articles)
    logx.Debug(reply, err)
    return reply, err

}

//
// BotTemplate
//  @Description:
//  @receiver this
//  @param key
//  @param template
//  @return error
//
func (this wechatUseCase) BotTemplate(key string, template *request.GroupRobotMsgTemplateCard) error {

    reply, err := this.wework.GroupRobotMessenger.SendTemplateCard(context.TODO(), key, template)
    logx.Debug(reply, err)
    return err

}
