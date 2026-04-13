package monitor

import (
	monitorlogs "github.com/ArtisanCloud/PowerX/internal/service/monitor_logs"
)

type handler struct {
	svc *monitorlogs.Service
}

func NewHandler() *handler {
	return &handler{svc: monitorlogs.NewService()}
}
