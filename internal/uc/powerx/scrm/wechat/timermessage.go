package wechat

import (
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/power"
	creq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/messageTemplate/request"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/request"
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
func (this *wechatUseCase) InvokeTimerMessageGrabUniteSend(ttp TimerTypeByte, sendTime int64) (err error) {

	key := fmt.Sprintf(HRedisScrmGroupMessageKey, ttp)

	vals, _ := this.kv.Hget(key, strconv.Itoa(int(sendTime)))
	if vals == `` {
		return nil
	}
	switch ttp {

	case AppGroupOrganizationMessageTimerTypeByte:
		err = this.callAppGroupOrganizationMessage(key, sendTime, vals)

	case AppMessageTimerTypeByte:
		err = this.callAppMessage(key, sendTime, vals)

	case AppGroupCustomerMessageTimerTypeByte:
		err = this.callCustomerGroupMessage(key, sendTime, vals)
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

//
// callCustomerGroupMessage
//  @Description:
//  @receiver this
//  @param key
//  @param sendTime
//  @param val
//  @return error
//
func (this *wechatUseCase) callCustomerGroupMessage(key string, sendTime int64, val string) error {

	message := &creq.RequestAddMsgTemplate{}
	err := json.Unmarshal([]byte(val), &message)
	if err == nil {
		_, err = this.PushWoWorkCustomerTemplateRequest(message, sendTime)
		_, err = this.kv.Hdel(key, strconv.Itoa(int(sendTime)))
	}

	return err

}
