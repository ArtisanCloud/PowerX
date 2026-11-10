package storagegov

import "context"

// DatabaseUsage 与目录计量是两种视角，不能再加到 df 已使用量。WAL 是共享集群量。
type DatabaseUsage struct {
	State         string          `json:"state"`
	ClusterID     string          `json:"cluster_id,omitempty"`
	InstanceID    string          `json:"instance_id"`
	Environment   string          `json:"environment"`
	DatabaseBytes *int64          `json:"database_bytes"`
	WALBytes      *int64          `json:"wal_bytes"`
	WALState      string          `json:"wal_state"`
	ReusableBytes *int64          `json:"reusable_bytes"` // 未做有成本的页扫描，不从 dead tuple 行数伪造字节。
	Relations     []RelationUsage `json:"relations"`
	Problem       string          `json:"problem,omitempty"`
}

type RelationUsage struct {
	ID           string `json:"id"`
	State        string `json:"state"`
	HeapBytes    *int64 `json:"heap_bytes"`
	IndexesBytes *int64 `json:"indexes_bytes"`
	ToastBytes   *int64 `json:"toast_bytes"`
	TotalBytes   *int64 `json:"total_bytes"`
	Problem      string `json:"problem,omitempty"`
}

type DatabaseCollector interface {
	CollectDatabase(context.Context) DatabaseUsage
}
