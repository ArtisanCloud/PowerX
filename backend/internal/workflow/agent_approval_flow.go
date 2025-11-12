package workflow

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// AgentApprovalFlow 简化版本的审批流。
type AgentApprovalFlow struct {
	mu      sync.Mutex
	tickets map[string]string
}

const (
	ticketStatusPending  = "pending"
	ticketStatusApproved = "approved"
	ticketStatusRejected = "rejected"
)

// NewAgentApprovalFlow 构造默认审批流。
func NewAgentApprovalFlow() *AgentApprovalFlow {
	return &AgentApprovalFlow{
		tickets: make(map[string]string),
	}
}

// Start 创建审批单并返回 ticket ID。
func (f *AgentApprovalFlow) Start(_ context.Context) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uuid.NewString()
	f.tickets[id] = ticketStatusPending
	return id
}

// Approve 标记审批通过。
func (f *AgentApprovalFlow) Approve(_ context.Context, ticketID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tickets[ticketID] = ticketStatusApproved
}

// Reject 标记审批拒绝。
func (f *AgentApprovalFlow) Reject(_ context.Context, ticketID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tickets[ticketID] = ticketStatusRejected
}

// Status 返回当前状态。
func (f *AgentApprovalFlow) Status(ticketID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tickets[ticketID]
}
