package supervisor

import "time"

type ProcState string

const (
	ProcStopped   ProcState = "stopped"
	ProcStarting  ProcState = "starting"
	ProcRunning   ProcState = "running"
	ProcUnhealthy ProcState = "unhealthy"
	ProcExited    ProcState = "exited"
	ProcCrashed   ProcState = "crashed"
)

// DynamicBindPlaceholder 用于在未显式指定 PX_BIND_ADDR 时占位，启动时由 supervisor 替换为实际端口。
const DynamicBindPlaceholder = "__PX_DYNAMIC_PORT__"

type Options struct {
	// 健康检查
	HealthPath     string        // 例如 "/healthz"
	HealthInterval time.Duration // 例如 2s
	HealthTimeout  time.Duration // 例如 1s

	// 自愈策略
	AutoRestart bool          // 默认 true
	BackoffBase time.Duration // 例如 1s
	BackoffMax  time.Duration // 例如 10s
}

type ProcInfo struct {
	ID        string    `json:"id"`
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	State     ProcState `json:"state"`
	Healthy   bool      `json:"healthy"`
	Restarts  int       `json:"restarts"`
	StartedAt time.Time `json:"started_at"`
	StoppedAt time.Time `json:"stopped_at,omitempty"`

	LastExitErr   string `json:"last_exit_err,omitempty"`
	HealthFails   int    `json:"health_fails"`
	HealthOKCount int    `json:"health_ok_count"`
}
