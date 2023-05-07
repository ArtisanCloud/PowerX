package wework

import (
	weworkLogic "PowerX/internal/logic/wework"
	"net/http"

	"PowerX/internal/svc"
)

func PostMessageHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := weworkLogic.NewWebhookPostMessageLogic(r.Context(), svcCtx)
		l.WebhookPostMessage(w, r)

	}
}
