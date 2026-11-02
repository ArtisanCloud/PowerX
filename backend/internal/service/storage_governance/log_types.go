package storagegov

import (
	"context"
	"time"
)

// LogControl 是现有轮转配置的只读说明，不建立第二套自动调度。
type LogControl struct {
	NativeManaged  bool   `json:"native_managed,omitempty"`
	Supported      bool   `json:"supported"`
	RetentionDays  int    `json:"retention_days"`
	RetentionCount int    `json:"retention_count"`
	BlockedReason  string `json:"blocked_reason,omitempty"`
}

// LogCandidate 只在服务端持久化；HTTP DTO 不返回 inode、路径或内部快照。
type LogCandidate struct {
	Name       string `json:"name"`
	Device     uint64 `json:"device"`
	Inode      uint64 `json:"inode"`
	Size       int64  `json:"size"`
	Blocks     int64  `json:"blocks"`
	ModifiedNS int64  `json:"modified_ns"`
	ChangedNS  int64  `json:"changed_ns"`
	Outcome    string `json:"outcome"`
}

type LogScan struct {
	Fingerprint string         `json:"fingerprint"`
	Candidates  []LogCandidate `json:"candidates"`
	Skipped     map[string]int `json:"skipped"`
	Complete    bool           `json:"complete"`
}

type LogLifecycle interface {
	Control(string) *LogControl
	Inspect(context.Context, string, time.Time) (LogScan, error)
	WithLock(context.Context, string, func() error) error
	Remove(context.Context, string, LogCandidate) (string, error)
}
