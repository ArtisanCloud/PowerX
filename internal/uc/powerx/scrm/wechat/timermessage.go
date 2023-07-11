package wechat

import (
    "encoding/json"
    "fmt"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/power"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/request"
    "github.com/zeromicro/go-zero/core/logx"
    "strconv"
)

//
// pushTimerMessageToKV
//  @Description:
//  @receiver this
//  @param ttp
//  @param sendTime
//  @param message
//
func (this *wechatUseCase) pushTimerMessageToKV(ttp TimerTypeByte, sendTime int64, message interface{}) {

    val := make(map[string]string)
    key := fmt.Sprintf(HRedisScrmGroupMessageKey, ttp)
    msg, _ := json.Marshal(message)
    val[strconv.Itoa(int(sendTime))] = string(msg)

    err := this.kv.HmsetCtx(this.ctx, key, val)
    if err != nil {
        panic(err)
    }

}

//
// InvokeTimerMessageGrabUniteSend
//  @Description: todo
//  @receiver this
//  @param ttp
//  @param sendTime
//  @return error
//
func (this *wechatUseCase) InvokeTimerMessageGrabUniteSend(ttp TimerTypeByte, sendTime int64) error {

    key := fmt.Sprintf(HRedisScrmGroupMessageKey, ttp)
    vals, err := this.kv.Hget(key, strconv.Itoa(int(sendTime)))
    if err != nil || vals == `` {
        logx.Errorf(`scrm.wework.timer.call.redis.error %v.`, err)
        return nil
    }
    switch ttp {

    case AppGroupOrganizationMessageTimerTypeByte:
        err = this.callAppGroupOrganizationMessage(key, sendTime, vals)

    case AppMessageTimerTypeByte:
        err = this.callAppMessage(key, sendTime, vals)

    case AppGroupCustomerMessageTimerTypeByte:

    }

    return err

}

//
// callAppGroupOrganizationMessage
//  @Description:
//  @receiver this
//  @param key
//  @param sendTime
//  @param vals
//  @return error
//
func (this *wechatUseCase) callAppGroupOrganizationMessage(key string, sendTime int64, val string) error {

    message := &power.HashMap{}
    err := json.Unmarshal([]byte(val), &message)
    if err == nil {
        _, err = this.PushAppWeWorkGroupMessageArticlesRequest(message, sendTime)
        _, err = this.kv.Hdel(key, strconv.Itoa(int(sendTime)))
    }
    return err
}

//
// callAppMessage
//  @Description:
//  @receiver this
//  @param key
//  @param sendTime
//  @param vals
//  @return error
//
func (this *wechatUseCase) callAppMessage(key string, sendTime int64, val string) error {

    message := &request.RequestMessageSendNews{}
    err := json.Unmarshal([]byte(val), &message)
    if err == nil {
        _, err = this.PushAppWeWorkMessageArticlesRequest(message, sendTime)
        _, err = this.kv.Hdel(key, strconv.Itoa(int(sendTime)))
    }

    return err

}
