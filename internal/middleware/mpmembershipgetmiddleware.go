package middleware

import "net/http"

type MPMembershipGetMiddleware struct {
}

func NewMPMembershipGetMiddleware() *MPMembershipGetMiddleware {
	return &MPMembershipGetMiddleware{}
}

func (m *MPMembershipGetMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO generate middleware implement function, delete after code implementation

		// Passthrough to next handler if need
		next(w, r)
	}
}
