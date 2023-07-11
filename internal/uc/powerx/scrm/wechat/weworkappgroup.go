package wechat

import (
    "PowerX/internal/model/scrm/app"
    "encoding/json"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/power"
    kresp "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/response"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/appChat/request"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/appChat/response"
    "time"
)

//
// CreateWeWorkAppGroupRequest
//  @Description:
//  @receiver this
//  @param option
//  @return reply
//  @return err
//
func (this wechatUseCase) CreateWeWorkAppGroupRequest(option *request.RequestAppChatCreate) (reply *response.ResponseAppChatCreate, err error) {

    reply, err = this.wework.MessageAppChat.Create(this.ctx, option)
    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.wechat.app.group.create.error`, reply.ResponseWork)
    }
    if err == nil {
        users, _ := json.Marshal(option.UserList)
        this.modelWeworkApp.group.Action(this.db, []*app.WeWorkAppGroup{
            {
                Name:     option.Name,
                Owner:    option.Owner,
                UserList: string(users),
                ChatID:   reply.ChatID,
            },
        })
    }
    return reply, err

}

//
// UpdateWeWorkAppGroupRequest
//  @Description:
//  @receiver this
//  @param option
//  @return error
//
func (this wechatUseCase) UpdateWeWorkAppGroupRequest(option *request.RequestAppChatUpdate) error {

    reply, err := this.wework.MessageAppChat.Update(this.ctx, option)
    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.wework.app.group.update.error`, *reply)
    }
    return err

}

//
// PullListWeWorkAppGroupRequest
//  @Description:
//  @receiver this
//  @param chatIDs
//  @return replys
//  @return err
//
func (this wechatUseCase) PullListWeWorkAppGroupRequest(chatIDs ...string) (replys []*power.HashMap, err error) {

    if chatIDs == nil {
        groups := this.modelWeworkApp.group.Query(this.db)
        chatIDs = app.AdapterGroupSliceChatIDs(func(groups []*app.WeWorkAppGroup) (ids []string) {
            for _, group := range groups {
                ids = append(ids, group.ChatID)
            }
            return ids
        })(groups)
    }
    for _, id := range chatIDs {

        reply, err := this.wework.MessageAppChat.Get(this.ctx, id)
        if err != nil {
            panic(err)
        } else {
            err = this.help.error(`scrm.pull.wework.app.group.detail.error`, reply.ResponseWork)
        }
        // update local
        if err == nil {
            group := hash(*reply.ChatInfo).fromHashMapToAppGroup()
            this.modelWeworkApp.group.Action(this.db, []*app.WeWorkAppGroup{group})
            replys = append(replys, reply.ChatInfo)
        }
    }

    return replys, err

}

//
// PushAppWeWorkGroupMessageArticlesRequest
//  @Description:
//  @receiver this
//  @param messages
//  @return *response.ResponseWork
//  @return error
//
func (this *wechatUseCase) PushAppWeWorkGroupMessageArticlesRequest(messages *power.HashMap, sendTime int64) (reply *kresp.ResponseWork, err error) {

    if sendTime > time.Now().Unix() {

        this.pushTimerMessageToKV(AppGroupOrganizationMessageTimerTypeByte, sendTime, messages)

    } else {
        msg := *messages
        chatIds := msg[`chatIds`].([]interface{})
        for _, id := range chatIds {
            msg[`chatid`] = id
            reply, err = this.wework.MessageAppChat.Send(this.ctx, &msg)
            if err != nil {
                panic(err)
            } else {
                err = this.help.error(`scrm.push.wework.app.group.message.articles.error`, *reply)
            }
        }

    }

    return reply, err

}

//
// fromHashMapToAppGroup
//  @Description:
//  @receiver this
//  @param obj
//
func (hash hash) fromHashMapToAppGroup() *app.WeWorkAppGroup {

    users, _ := json.Marshal(hash[`userlist`])
    return &app.WeWorkAppGroup{
        Name:     hash[`name`].(string),
        Owner:    hash[`owner`].(string),
        UserList: string(users),
        ChatID:   hash[`chatid`].(string),
    }

}
