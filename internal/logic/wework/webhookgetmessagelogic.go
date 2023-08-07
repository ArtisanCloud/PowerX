package customer

import (
	"PowerX/internal/svc"
	"PowerX/pkg/httpx"
	"context"
	"github.com/zeromicro/go-zero/core/logx"
	"net/http"
)

type WebhookGetMessageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewWebhookGetMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WebhookGetMessageLogic {
	return &WebhookGetMessageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *WebhookGetMessageLogic) WebhookGetMessage(w http.ResponseWriter, r *http.Request) {
	//fmt.DD(req)
	//body, _ := io.ReadAll(r.Body)
	//fmt.Dump(string(body))
	//
	//defer r.Body.Close()
	//rs, err := l.svcCtx.PowerX.WeWork.API.Server.Serve(r)
	rs, err := l.svcCtx.PowerX.SCRM.Wework.Server.Serve(r)
	if err != nil {
		panic(err)
	}

	err = httpx.HttpResponseSend(rs, w)

	if err != nil {
		panic(err)
	}
	return
}
