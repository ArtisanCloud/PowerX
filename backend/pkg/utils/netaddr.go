package utils

// utils/netaddr.go

import (
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"net"
	"os"
	"strings"
)

func ResolveAddrWithDynamicPort(addr string, preferEnv string, fallbackHost string) string {
	s := strings.TrimSpace(addr)
	if s == supervisor.DynamicBindPlaceholder || s == ":"+supervisor.DynamicBindPlaceholder {
		port := strings.TrimSpace(os.Getenv(preferEnv))
		if port == "" {
			port = strings.TrimSpace(os.Getenv("PORT"))
		}
		if port == "" {
			port = "0" // 让内核分配
		}
		if strings.HasPrefix(s, ":") || fallbackHost == "" {
			return ":" + port
		}
		if fallbackHost == "" {
			fallbackHost = "127.0.0.1"
		}
		return net.JoinHostPort(fallbackHost, port)
	}
	return s
}
