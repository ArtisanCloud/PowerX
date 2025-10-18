package audit

import (
	"context"
	"time"
)

// Record 描述一次发布或订阅调用的审计数据。
type Record struct {
	ID           string
	TenantID     string
	Topic        string
	PrincipalID  string
	Action       string
	Status       string
	LatencyMs    int64
	TraceID      string
	Metadata     map[string]string
	HappenedAt   time.Time
	ErrorMessage string
}

// Service 将审计记录写入外部审计服务或日志。
type Service interface {
	Write(ctx context.Context, record Record) error
}
