package pluginx

import (
	"github.com/gorilla/mux"
	"github.com/zeromicro/go-zero/rest"
	"net/http"
)

type MuxServer struct {
	*rest.Server
	router *mux.Router
}

func NewMuxServer(c rest.RestConf) *MuxServer {
	server := rest.MustNewServer(c)
	router := mux.NewRouter()
	server.AddRoutes([]rest.Route{
		{
			Method:  http.MethodGet,
			Path:    "/",
			Handler: router.ServeHTTP,
		},
	})

	return &MuxServer{
		Server: server,
		router: router,
	}
}

func (ms *MuxServer) Router() *mux.Router {
	return ms.router
}
