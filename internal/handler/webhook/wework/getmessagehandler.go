package wework

import (
	weworkLogic "PowerX/internal/logic/wework"
	"net/http"

	"PowerX/internal/svc"
)

func GetMessageHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := weworkLogic.NewWebhookGetMessageLogic(r.Context(), svcCtx)
		l.WebhookGetMessage(w, r)

	}
}
