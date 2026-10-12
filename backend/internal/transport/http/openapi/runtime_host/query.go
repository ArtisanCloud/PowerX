package runtime_host

import (
	"net/url"

	svc "github.com/ArtisanCloud/PowerX/internal/service/runtime_host"
	"github.com/gin-gonic/gin"
)

func urlQuery(c *gin.Context) (url.Values, error) {
	v, e := url.ParseQuery(c.Request.URL.RawQuery)
	if e != nil {
		return nil, svc.Invalid()
	}
	return v, nil
}
