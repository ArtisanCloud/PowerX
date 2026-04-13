package monitorlogs

import "time"

type Driver string

const (
	DriverLoki  Driver = "loki"
	DriverFile  Driver = "file"
	DriverStdio Driver = "stdio"
)

type Capabilities struct {
	SupportsLabelQuery  bool   `json:"supports_label_query"`
	SupportsTraceQuery  bool   `json:"supports_trace_query"`
	SupportsJobQuery    bool   `json:"supports_job_query"`
	SupportsPolicyQuery bool   `json:"supports_policy_query"`
	SupportsGrafanaLink bool   `json:"supports_grafana_link"`
	HistoryLimited      bool   `json:"history_limited"`
	LimitationNote      string `json:"limitation_note"`
}

type ConfigView struct {
	Driver         Driver       `json:"driver"`
	Capabilities   Capabilities `json:"capabilities"`
	GrafanaBaseURL string       `json:"grafana_base_url"`
}

type QueryRequest struct {
	TraceID  string
	JobID    uint64
	PolicyID uint64
	Keyword  string
	From     *time.Time
	To       *time.Time
	Page     int
	PageSize int
}

type Entry struct {
	Timestamp time.Time `json:"ts"`
	Level     string    `json:"level"`
	Module    string    `json:"module"`
	TraceID   string    `json:"trace_id"`
	JobID     uint64    `json:"job_id"`
	PolicyID  uint64    `json:"policy_id"`
	Message   string    `json:"message"`
	Raw       string    `json:"raw"`
}

type QueryMeta struct {
	Driver   Driver `json:"driver"`
	Degraded bool   `json:"degraded"`
	Hint     string `json:"hint"`
	Grafana  string `json:"grafana_url,omitempty"`
}

type QueryResult struct {
	Items []Entry
	Total int
	Meta  QueryMeta
}

type Provider interface {
	Driver() Driver
	Config() ConfigView
	Query(req QueryRequest) (QueryResult, error)
}
