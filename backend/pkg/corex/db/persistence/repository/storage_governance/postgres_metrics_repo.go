package storagegovrepo

import (
	"context"
	"errors"
	"gorm.io/gorm"
)

// RelationMetrics 为不相加重复的存储分项：HeapBytes（含 FSM/VM）、IndexesBytes、ToastBytes（含 TOAST 索引）。
type RelationMetrics struct {
	HeapBytes    int64
	IndexesBytes int64
	ToastBytes   int64
	TotalBytes   int64
}

type PostgresMetricsRepository struct{ db *gorm.DB }

func NewPostgresMetricsRepository(db *gorm.DB) *PostgresMetricsRepository {
	return &PostgresMetricsRepository{db: db}
}

func (r *PostgresMetricsRepository) DatabaseSize(ctx context.Context) (int64, error) {
	var size int64
	err := r.db.WithContext(ctx).Raw("SELECT pg_catalog.pg_database_size(current_database())").Scan(&size).Error
	return size, err
}

// Relation 仅接收部署端 allowlist 的 schema/name，不拼接标识符，不扫描业务行或安装扩展。
func (r *PostgresMetricsRepository) Relation(ctx context.Context, schema, name string) (RelationMetrics, error) {
	var row RelationMetrics
	q := r.db.WithContext(ctx).Raw(`SELECT pg_catalog.pg_table_size(c.oid) - CASE WHEN c.reltoastrelid = 0 THEN 0 ELSE pg_catalog.pg_total_relation_size(c.reltoastrelid) END AS heap_bytes,
		pg_catalog.pg_indexes_size(c.oid) AS indexes_bytes,
		CASE WHEN c.reltoastrelid = 0 THEN 0 ELSE pg_catalog.pg_total_relation_size(c.reltoastrelid) END AS toast_bytes,
		pg_catalog.pg_total_relation_size(c.oid) AS total_bytes
		FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = ? AND c.relname = ? AND c.relkind IN ('r','m')`, schema, name).Scan(&row)
	if q.Error != nil {
		return row, q.Error
	}
	if q.RowsAffected != 1 {
		return row, gorm.ErrRecordNotFound
	}
	if row.HeapBytes < 0 || row.IndexesBytes < 0 || row.ToastBytes < 0 || row.TotalBytes != row.HeapBytes+row.IndexesBytes+row.ToastBytes {
		return row, errors.New("inconsistent relation metrics")
	}
	return row, nil
}

// WAL 为整个集群共享的实际文件量，不分摊给某个环境；权限不足向上返回失败。
func (r *PostgresMetricsRepository) WALSize(ctx context.Context) (int64, error) {
	var size int64
	err := r.db.WithContext(ctx).Raw("SELECT COALESCE(sum(size), 0)::bigint FROM pg_catalog.pg_ls_waldir()").Scan(&size).Error
	return size, err
}
