package capabilityregistry

import (
	"context"
	"sync"

	discovery "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
)

// SnapshotApplier 定义跨区域复制时的快照落地接口。
type SnapshotApplier interface {
	ApplyReplica(ctx context.Context, snapshot discovery.Snapshot) error
}

// Replicator 将快照广播到多个下游区域。
type Replicator struct {
	mu       sync.RWMutex
	appliers []SnapshotApplier
}

// NewReplicator 创建跨区域复制器。
func NewReplicator(appliers ...SnapshotApplier) *Replicator {
	cloned := make([]SnapshotApplier, len(appliers))
	copy(cloned, appliers)
	return &Replicator{appliers: cloned}
}

// AddApplier 动态新增下游区域。
func (r *Replicator) AddApplier(applier SnapshotApplier) {
	if applier == nil {
		return
	}
	r.mu.Lock()
	r.appliers = append(r.appliers, applier)
	r.mu.Unlock()
}

// Replicate 将快照复制到所有下游，遇到首个错误即停止并返回该错误。
func (r *Replicator) Replicate(ctx context.Context, snapshots []discovery.Snapshot) error {
	r.mu.RLock()
	receivers := make([]SnapshotApplier, len(r.appliers))
	copy(receivers, r.appliers)
	r.mu.RUnlock()

	for _, applier := range receivers {
		if applier == nil {
			continue
		}
		for _, snapshot := range snapshots {
			if err := applier.ApplyReplica(ctx, snapshot); err != nil {
				return err
			}
		}
	}
	return nil
}
