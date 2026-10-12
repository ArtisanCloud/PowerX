package backup_ops

import (
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// RestoreCopyClientOptions 仅描述客户端连接入口，不改变恢复任务实际使用的数据库。
// 未显式传入时，从 POWERX_OPS_RESTORE_* 环境变量加载一次。
type RestoreCopyClientOptions struct {
	AccessMode      string
	Host            string
	Port            uint16
	SSLMode         string
	TunnelHost      string
	TunnelPort      uint16
	RequireExplicit bool
}

// RestoreCopySSHTarget 是 SSH 服务器视角下的 PostgreSQL 转发目标。
// SSH 登录地址和凭据由管理员自行提供，不推测浏览器域名或返回私钥。
type RestoreCopySSHTarget struct {
	Host string `json:"host"`
	Port uint16 `json:"port"`
}

type restoreCopyClientEndpoint struct {
	AccessMode string
	Host       string
	Port       uint16
	SSLMode    string
	Tunnel     *RestoreCopySSHTarget
}

func restoreCopyClientConfigError() error {
	return restoreFailure(503, "RESTORE_CONNECTION_CONFIG_INVALID", "恢复副本客户端连接配置无效；请核对 POWERX_OPS_RESTORE_* 的访问方式、主机、端口及 SSL 设置")
}

func restoreCopyClientOptionsFromEnv() (RestoreCopyClientOptions, error) {
	opts := RestoreCopyClientOptions{
		AccessMode:      os.Getenv("POWERX_OPS_RESTORE_ACCESS_MODE"),
		Host:            os.Getenv("POWERX_OPS_RESTORE_CLIENT_HOST"),
		SSLMode:         os.Getenv("POWERX_OPS_RESTORE_CLIENT_SSL_MODE"),
		TunnelHost:      os.Getenv("POWERX_OPS_RESTORE_TUNNEL_HOST"),
		RequireExplicit: strings.EqualFold(strings.TrimSpace(os.Getenv("POWERX_MODE")), "docker"),
	}
	for _, field := range []struct {
		name string
		port *uint16
	}{
		{"POWERX_OPS_RESTORE_CLIENT_PORT", &opts.Port},
		{"POWERX_OPS_RESTORE_TUNNEL_PORT", &opts.TunnelPort},
	} {
		if value := strings.TrimSpace(os.Getenv(field.name)); value != "" {
			port, err := strconv.ParseUint(value, 10, 16)
			if err != nil || port == 0 {
				return opts, restoreCopyClientConfigError()
			}
			*field.port = uint16(port)
		}
	}
	return opts, nil
}

func (s *RestoreCopyService) connectionEndpoint() (restoreCopyClientEndpoint, error) {
	if s.clientConfigErr != nil {
		return restoreCopyClientEndpoint{}, s.clientConfigErr
	}
	sslMode := "prefer"
	if env, err := postgresEnvironment(sourceDSN(s.db)); err == nil {
		for _, value := range env {
			if strings.HasPrefix(value, "PGSSLMODE=") {
				sslMode = strings.TrimPrefix(value, "PGSSLMODE=")
			}
		}
	}
	return resolveRestoreCopyClientEndpoint(s.cfg, s.clientConnection, sslMode)
}

func resolveRestoreCopyClientEndpoint(cfg *pgx.ConnConfig, opts RestoreCopyClientOptions, sslMode string) (restoreCopyClientEndpoint, error) {
	mode := strings.TrimSpace(opts.AccessMode)
	if mode == "" {
		mode = "direct"
	}
	host, port := strings.TrimSpace(opts.Host), opts.Port
	tunnelHost := strings.TrimSpace(opts.TunnelHost)
	if value := strings.TrimSpace(opts.SSLMode); value != "" {
		sslMode = value
	}
	invalid := func() (restoreCopyClientEndpoint, error) {
		return restoreCopyClientEndpoint{}, restoreCopyClientConfigError()
	}
	if mode != "direct" && mode != "ssh_tunnel" {
		return invalid()
	}
	switch sslMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return invalid()
	}
	if (host == "") != (port == 0) {
		return invalid()
	}
	if mode == "direct" {
		if tunnelHost != "" || opts.TunnelPort != 0 {
			return invalid()
		}
		if host == "" {
			if opts.RequireExplicit || cfg == nil || strings.HasPrefix(cfg.Host, "/") {
				return restoreCopyClientEndpoint{}, restoreFailure(409, "RESTORE_CONNECTION_NOT_CONFIGURED", "尚未配置恢复副本的客户端访问入口；容器内部地址不能直接用于 TablePlus，请配置客户端地址或受限端口映射及 SSH 隧道")
			}
			// systemctl/原生部署沿用现有地址，远程回环地址仍需管理员建立隧道。
			host, port = cfg.Host, cfg.Port
		}
	} else {
		if !restoreCopyLoopbackHost(host) || port == 0 || !restoreCopyValidHost(tunnelHost) || opts.TunnelPort == 0 {
			return invalid()
		}
	}
	if !restoreCopyValidHost(host) || port == 0 {
		return invalid()
	}
	endpoint := restoreCopyClientEndpoint{AccessMode: mode, Host: host, Port: port, SSLMode: sslMode}
	if mode == "ssh_tunnel" {
		endpoint.Tunnel = &RestoreCopySSHTarget{Host: tunnelHost, Port: opts.TunnelPort}
	}
	return endpoint, nil
}

func restoreCopyLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// 仅接受裸主机/IP，拒绝 URL、路径、用户信息、多主机和通配监听地址。
func restoreCopyValidHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsUnspecified()
	}
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}
