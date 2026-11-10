package taskqueue

import (
	"time"
)

const RuntimeContract = "taskqueue.v2"
const RuntimeEndpoint = "core://runtime/taskqueue"
const EnqueueCapability = "com.corex.runtime.taskqueue.service_enqueue"
const ConsumeCapability = "com.corex.runtime.taskqueue.service_consume"

type RuntimeLease struct {
	ReceiptUUID string    `json:"receipt_uuid"`
	Fence       uint64    `json:"fence"`
	ExpiresAt   time.Time `json:"expires_at"`
}
type RuntimeMessage struct {
	ID             string            `json:"id"`
	TenantKey      string            `json:"tenant_key"`
	SubscriberID   string            `json:"subscriber_id"`
	Topic          string            `json:"topic"`
	PayloadBase64  string            `json:"payload_base64"`
	Headers        map[string]string `json:"headers,omitempty"`
	TraceID        string            `json:"trace_id,omitempty"`
	VisibleAt      *time.Time        `json:"visible_at,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	IdempotencyKey string            `json:"idempotency_key"`
	Attempt        int               `json:"attempt,omitempty"`
	Lease          *RuntimeLease     `json:"lease,omitempty"`
}
type RuntimeRequest struct {
	Outcome         string            `json:"outcome,omitempty"`
	ContractVersion string            `json:"contract_version"`
	Operation       string            `json:"operation"`
	TenantKey       string            `json:"tenant_key"`
	Message         *RuntimeMessage   `json:"message,omitempty"`
	SubscriberID    string            `json:"subscriber_id,omitempty"`
	WorkerID        string            `json:"worker_id,omitempty"`
	MaxItems        int               `json:"max_items,omitempty"`
	WaitTimeoutMS   int64             `json:"wait_timeout_ms,omitempty"`
	MessageID       string            `json:"message_id,omitempty"`
	ReceiptUUID     string            `json:"receipt_uuid,omitempty"`
	RetryAt         *time.Time        `json:"retry_at,omitempty"`
	Reason          string            `json:"reason,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}
type RuntimeScope struct {
	TenantUUID    string
	SourceService string
	EnvironmentID string
}
