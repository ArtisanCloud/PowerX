package customer

import (
	"PowerX/internal/svc"
	"PowerX/pkg/httpx"
	fmt "PowerX/pkg/printx"
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/kernel"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/contract"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/models"
	models2 "github.com/ArtisanCloud/PowerWeChat/v3/src/work/server/handlers/models"
	"github.com/zeromicro/go-zero/core/logx"
	"net/http"
)

type WebhookPostMessageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewWebhookPostMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WebhookPostMessageLogic {
	return &WebhookPostMessageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *WebhookPostMessageLogic) WebhookPostMessage(w http.ResponseWriter, r *http.Request) {

	// 这里使用了PowerWechat的SDK，来处理解密企业微信的消息
	// 请确保企业微信的相关参数配置已经配置正确
	rs, err := l.svcCtx.PowerX.WeWork.API.Server.Notify(r, func(event contract.EventInterface) interface{} {
		fmt.Dump("event", event)
		//return  "handle callback"

		switch event.GetMsgType() {
		case models.CALLBACK_MSG_TYPE_TEXT:
			msg := models2.MessageText{}
			err := event.ReadMessage(&msg)
			if err != nil {
				println(err.Error())
				return "error"
			}
			fmt.Dump(msg)

		}

		return kernel.SUCCESS_EMPTY_RESPONSE

	})
	if err != nil {
		panic(err)
	}

	err = httpx.HttpResponseSend(rs, w)

	if err != nil {
		panic(err)
	}
	return

}
