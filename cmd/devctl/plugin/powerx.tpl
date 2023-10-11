package powerx

import (
	"PluginTemplate/pkg/powerx/client"
)

type PowerX struct {
	*client.PClient
	{{range .}}
	{{.}} *{{.}}
	{{end}}
}

func NewPowerX(endpoint string, debug bool) *PowerX {
	power := &PowerX{
		PClient: client.NewPClient(endpoint, debug),
	}
	{{range .}}
	power.{{.}} = &{{.}}{power}
	{{end}}
	return power
}
