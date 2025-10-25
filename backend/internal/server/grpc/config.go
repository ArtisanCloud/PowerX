package grpcserver

import (
	"fmt"
	"time"
)

type GRPCConfig struct {
	Enable     bool   `yaml:"enable"`      // 是否启用 gRPC（默认 true）
	Network    string `yaml:"network"`     // "tcp"
	Host       string `yaml:"host"`        // 监听地址，默认 0.0.0.0
	Port       int    `yaml:"port"`        // 监听端口，默认 9001
	Reflection bool   `yaml:"reflection"`  // 启用 gRPC 反射（开发强烈建议开）
	UseTLS     bool   `yaml:"useTLS"`      // 是否启用 TLS
	CertFile   string `yaml:"certFile"`    // 服务器证书
	KeyFile    string `yaml:"keyFile"`     // 服务器私钥
	Health     bool   `yaml:"health"`      // 启用健康检查服务
	MaxRecvMB  int    `yaml:"max_recv_mb"` // 单条消息最大接收（MiB）
	MaxSendMB  int    `yaml:"max_send_mb"` // 单条消息最大发送（MiB）

	Keepalive    KeepaliveConfig    `yaml:"keepalive"`
	TLS          *TLSConfig         `yaml:"tls,omitempty"` // 为空表示不启用 TLS
	Interceptors InterceptorsConfig `yaml:"interceptors"`  // 可按需开关
	Limits       LimitsConfig       `yaml:"limits"`        // 可选并发等限制

}

type KeepaliveConfig struct {
	Enable bool `yaml:"enable"`

	// 服务器参数（对应 grpc keepalive.ServerParameters）
	MaxConnectionIdle     time.Duration `yaml:"max_connection_idle"`      // e.g. "2m"
	MaxConnectionAge      time.Duration `yaml:"max_connection_age"`       // e.g. "0s"
	MaxConnectionAgeGrace time.Duration `yaml:"max_connection_age_grace"` // e.g. "30s"
	Time                  time.Duration `yaml:"time"`                     // 心跳间隔，e.g. "1m"
	Timeout               time.Duration `yaml:"timeout"`                  // 心跳超时，e.g. "5s"

	// 强制策略（对应 grpc keepalive.EnforcementPolicy）
	MinTime             time.Duration `yaml:"min_time"`              // 最小心跳间隔，e.g. "5s"
	PermitWithoutStream bool          `yaml:"permit_without_stream"` // 无活动流时是否允许心跳
}

type TLSConfig struct {
	Enable            bool   `yaml:"enable"`
	CertFile          string `yaml:"cert_file"`           // 服务器证书
	KeyFile           string `yaml:"key_file"`            // 服务器私钥
	RequireClientCert bool   `yaml:"require_client_cert"` // 是否开启 mTLS
	ClientCAFile      string `yaml:"client_ca_file"`      // 客户端 CA（mTLS 时必填）
}

type InterceptorsConfig struct {
	Logging    bool `yaml:"logging"`    // 访问日志
	Recovery   bool `yaml:"recovery"`   // panic 恢复
	Authz      bool `yaml:"authz"`      // 权限/鉴权
	Audit      bool `yaml:"audit"`      // 审计（who/what/when/PRN/trace）
	Tracing    bool `yaml:"tracing"`    // 分布式追踪
	Prometheus bool `yaml:"prometheus"` // 指标
}

type LimitsConfig struct {
	MaxConcurrentStreams uint32 `yaml:"max_concurrent_streams"` // 并发流上限（为空则使用 gRPC 默认）
}

// ---- 辅助方法 ----

func (c *GRPCConfig) Addr() string {
	host := c.Host
	if host == "" {
		host = "0.0.0.0"
	}
	port := c.Port
	if port == 0 {
		port = 9001
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func DefaultGRPCConfig() GRPCConfig {
	return GRPCConfig{
		Enable:     true,
		Network:    "tcp",
		Host:       "0.0.0.0",
		Port:       9001,
		Reflection: true,
		Health:     true,
		MaxRecvMB:  32, // 32 MiB
		MaxSendMB:  32, // 32 MiB
		Keepalive: KeepaliveConfig{
			Enable:                true,
			MaxConnectionIdle:     2 * time.Minute,
			MaxConnectionAge:      0,
			MaxConnectionAgeGrace: 30 * time.Second,
			Time:                  1 * time.Minute,
			Timeout:               5 * time.Second,
			MinTime:               5 * time.Second,
			PermitWithoutStream:   true,
		},
		TLS: nil, // 开发默认不启用 TLS
		Interceptors: InterceptorsConfig{
			Logging:    true,
			Recovery:   true,
			Authz:      true,
			Audit:      true,
			Tracing:    false,
			Prometheus: false,
		},
		Limits: LimitsConfig{
			MaxConcurrentStreams: 0, // 0 表示使用默认
		},
	}
}
