package pluginx

import (
	"net/http"
)

type ProxyRouter interface {
	ProxyHandleFunc() http.HandlerFunc
}
