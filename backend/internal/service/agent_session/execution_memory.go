package agent_session

import (
	"context"
	"encoding/json"
	"sync"
)

const MaxExecutionHistoryMessages = 128
const MaxExecutionHistoryBytes = 256 * 1024

type executionMemoryKey struct{}

// ExecutionMemory belongs to exactly one accepted invocation. The runtime can
// record structured pending state, but only Service commits it with success.
type ExecutionMemory struct {
	History []Message
	Pending map[string]any
	mu      sync.Mutex
	result  map[string]any
}

func ExecutionMemoryFromContext(ctx context.Context) *ExecutionMemory {
	m, _ := ctx.Value(executionMemoryKey{}).(*ExecutionMemory)
	return m
}

func (m *ExecutionMemory) SetPending(task map[string]any) error {
	if task != nil {
		id, idOK := task["task_id"].(string)
		ref, refOK := task["node_ref"].(string)
		if !idOK || id == "" || !refOK || ref == "" || task["status"] != "awaiting_params" {
			return ErrDependency
		}
	}
	raw, err := json.Marshal(task)
	if err != nil || len(raw) > MaxExecutionHistoryBytes {
		return ErrDependency
	}
	var clone map[string]any
	if err := json.Unmarshal(raw, &clone); err != nil {
		return ErrDependency
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if clone != nil && m.result != nil && m.result["task_id"] != clone["task_id"] {
		return ErrConflict
	}
	m.result = clone
	return nil
}

func (m *ExecutionMemory) CompleteTask(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.result != nil && m.result["task_id"] == taskID {
		m.result = nil
	}
}

func (m *ExecutionMemory) pendingJSON() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return json.Marshal(m.result)
}
