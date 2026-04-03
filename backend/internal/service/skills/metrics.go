package skills

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Metrics keeps lightweight counters for skill governance and invocation flows.
type Metrics struct {
	lifecycleCount uint64
	traceCount     uint64
	keys           sync.Map
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) IncLifecycle(action, skillID, version string) {
	if m == nil {
		return
	}
	atomic.AddUint64(&m.lifecycleCount, 1)
	m.storeLabelKey("lifecycle", action, skillID, version, "")
}

func (m *Metrics) IncTrace(status, skillID, version, tenantUUID string) {
	if m == nil {
		return
	}
	atomic.AddUint64(&m.traceCount, 1)
	m.storeLabelKey("trace", status, skillID, version, tenantUUID)
}

func (m *Metrics) storeLabelKey(kind, status, skillID, version, tenantUUID string) {
	key := fmt.Sprintf("%s|%s|%s|%s|%s", kind, status, skillID, version, tenantUUID)
	m.keys.Store(key, struct{}{})
}
