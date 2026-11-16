package capability_registry

import (
	"context"
	"errors"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
)

// RetentionScanKeys 限制进入期限与关联核验的主键数量，而非仅限制最终命中的数量。
const RetentionScanKeys = 2048

// RetentionPosition 为两张固定审计表的主键位置，不接受外部表名或过滤条件。
type RetentionPosition struct {
	Traces int64 `json:"traces"`
	Events int64 `json:"events"`
}

// RetentionWindow 将一次预览和确认执行限定在同一个主键窗口和轮次上界。
type RetentionWindow struct {
	Before    RetentionPosition `json:"before"`
	End       RetentionPosition `json:"end"`
	Highwater RetentionPosition `json:"highwater"`
}

// Valid 检查窗口单调性；实际主键数量在执行前重新核验。
func (w RetentionWindow) Valid() bool {
	return w.Before.Traces >= 0 && w.Before.Events >= 0 &&
		w.Before.Traces <= w.End.Traces && w.End.Traces <= w.Highwater.Traces &&
		w.Before.Events <= w.End.Events && w.End.Events <= w.Highwater.Events
}

// RetentionProgress 区分本批没有候选和本轮已检查完；Next 只在事务提交后生效。
type RetentionProgress struct {
	Window   RetentionWindow   `json:"window"`
	Next     RetentionPosition `json:"next"`
	Complete bool              `json:"complete"`
}

// Valid 确认游标属于该窗口，完成标志与冻结上界一致。
func (p RetentionProgress) Valid() bool {
	w := p.Window
	return w.Valid() && p.Next.Traces >= w.Before.Traces && p.Next.Traces <= w.End.Traces &&
		p.Next.Events >= w.Before.Events && p.Next.Events <= w.End.Events &&
		p.Complete == (p.Next == w.Highwater)
}

// ScanHighwater 固定本轮上界，新写入记录留到下一轮；MAX 使用既有主键索引。
func (r *RetentionRepository) ScanHighwater(ctx context.Context) (RetentionPosition, error) {
	var p RetentionPosition
	for _, v := range []struct {
		table string
		value *int64
	}{{m.InvocationTrace{}.TableName(), &p.Traces}, {m.CapabilityEventPublication{}.TableName(), &p.Events}} {
		if err := r.db.WithContext(ctx).Table(v.table).Select("COALESCE(MAX(id),0)").Scan(v.value).Error; err != nil {
			return p, err
		}
	}
	return p, nil
}

func (r *RetentionRepository) scanKeys(ctx context.Context, table string, after, end int64) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Table(table).Select("id").Where("id > ? AND id <= ?", after, end).
		Order("id").Limit(RetentionScanKeys+1).Pluck("id", &ids).Error
	return ids, err
}

// ScanWindow 按实际存在的主键向前推进，避免稀疏 ID 令扫描长期停留在空洞中。
func (r *RetentionRepository) ScanWindow(ctx context.Context, after, high RetentionPosition) (RetentionWindow, error) {
	w := RetentionWindow{Before: after, End: high, Highwater: high}
	if !w.Valid() {
		return w, errors.New("invalid_retention_window")
	}
	for _, v := range []struct {
		table string
		after int64
		end   *int64
	}{{m.InvocationTrace{}.TableName(), after.Traces, &w.End.Traces}, {m.CapabilityEventPublication{}.TableName(), after.Events, &w.End.Events}} {
		ids, err := r.scanKeys(ctx, v.table, v.after, *v.end)
		if err != nil {
			return w, err
		}
		if len(ids) > RetentionScanKeys {
			*v.end = ids[RetentionScanKeys-1]
		}
	}
	return w, nil
}

// IsRetentionQueryTimeout 只识别类型化取消／超时，不向页面暴露数据库错误或 SQL。
func IsRetentionQueryTimeout(err error) bool {
	var state interface{ SQLState() string }
	return errors.Is(err, context.DeadlineExceeded) || errors.As(err, &state) && state.SQLState() == "57014"
}

// Batch 兼容单批调用；持续治理必须使用 BatchWindow 并保存返回的扫描进度。
func (r *RetentionRepository) Batch(ctx context.Context, c RetentionCutoffs, apply bool) (RetentionBatch, error) {
	high, err := r.ScanHighwater(ctx)
	if err != nil {
		return RetentionBatch{}, err
	}
	w, err := r.ScanWindow(ctx, RetentionPosition{}, high)
	if err != nil {
		return RetentionBatch{}, err
	}
	counts, _, err := r.BatchWindow(ctx, c, apply, w)
	return counts, err
}
