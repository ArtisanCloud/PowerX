package payment

import (
	paymentLogic "PowerX/internal/logic/wx/payment"
	"net/http"

	"PowerX/internal/svc"
)

func PostMessageHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := paymentLogic.NewWXPostPaymentLogic(r.Context(), svcCtx)
		l.WebhookWXPostPayment(w, r)

	}
}
