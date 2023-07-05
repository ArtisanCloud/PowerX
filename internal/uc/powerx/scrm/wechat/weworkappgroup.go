package wechat

import (
    "PowerX/internal/model/scrm/app"
    "encoding/json"
    "fmt"
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
        group := hash(*reply.ChatInfo).fromHashMapToAppGroup()
        this.modelWeworkApp.group.Action(this.db, []*app.WeWorkAppGroup{group})

        replys = append(replys, reply.ChatInfo)

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

        this.hashPushToAppGroupSendQueue(messages, sendTime)

    } else {
        reply, err = this.wework.MessageAppChat.Send(this.ctx, messages)
        if err != nil {
            panic(err)

        } else {
            err = this.help.error(`scrm.push.wework.app.group.message.articles.error`, *reply)
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

var (
    HRedisScrmGroupMessageKey = `scrm:app:group:%d`
)

//
// hashPushToAppGroupSendQueue
//  @Description:
//  @receiver this
//  @param message
//  @param sendTime
//
func (this *wechatUseCase) hashPushToAppGroupSendQueue(message *power.HashMap, sendTime int64) {

    key := fmt.Sprintf(HRedisScrmGroupMessageKey, sendTime)
    val := make(map[string]string)
    msg, _ := json.Marshal(message)
    val[(*message)[`chatid`].(string)] = string(msg)
    err := this.kv.HmsetCtx(this.ctx, key, val)
    if err != nil {
        panic(err)
    }

}

//
// InvokeAppGroupMessageCaches
//  @Description:
//  @receiver this
//  @param sendTime
//  @return msg
//
func (this *wechatUseCase) InvokeAppGroupMessageCaches(sendTime int64) (count int) {

    key := fmt.Sprintf(HRedisScrmGroupMessageKey, sendTime)
    vals, err := this.kv.Hgetall(key)
    if err != nil {
        panic(err)
    }

    for _, val := range vals {
        message := &power.HashMap{}
        err := json.Unmarshal([]byte(val), &message)
        if err == nil {
            _, _ = this.PushAppWeWorkGroupMessageArticlesRequest(message, sendTime)
            _, _ = this.kv.Hdel(key, (*message)[`chatid`].(string))
            count++
        }

    }

    return count

}
