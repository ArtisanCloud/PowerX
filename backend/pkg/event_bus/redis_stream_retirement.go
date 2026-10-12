package event_bus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const StreamRetired StreamNackOutcome = "retired"

func validRetiredRunID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id
}

// RetireRun 仅在调用方确认终态归档及 SQL locator 后调用。
// 不对共享队列设置 TTL；按完整 Run ID 移除残留投递，其他运行不受影响。
func (d *RedisStreamTaskDriver) RetireRun(ctx context.Context, tenant, subscriber, group, runID string, messageIDs []string, at time.Time) error {
	if d == nil || d.client == nil || tenant == "" || subscriber == "" || group == "" || !validRetiredRunID(runID) || at.IsZero() {
		return fmt.Errorf("invalid retired run")
	}
	stream := taskStreamKey(tenant, subscriber)
	base := strings.TrimSuffix(stream, ":stream")
	if err := d.client.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	// 先关闭该 Run 的新投递/NACK/延迟提升，再扫描旧记录；可重复恢复。
	if err := d.client.ZAdd(ctx, base+":retired_runs", redis.Z{Score: float64(at.UnixMilli()), Member: runID}).Err(); err != nil {
		return err
	}
	if err := d.client.ZRemRangeByScore(ctx, base+":retired_runs", "-inf", fmt.Sprint(time.Now().UnixMilli())).Err(); err != nil {
		return err
	}
	for _, key := range []string{stream, base + ":dead"} {
		cursor := "-"
		for {
			entries, err := d.client.XRangeN(ctx, key, cursor, "+", 200).Result()
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				break
			}
			for _, entry := range entries {
				raw, ok := entry.Values["message"].(string)
				if !ok {
					return fmt.Errorf("task record lacks message")
				}
				var m TaskMessage
				if err := json.Unmarshal([]byte(raw), &m); err != nil {
					return err
				}
				if m.TenantKey != tenant || m.SubscriberID != subscriber || !strings.HasPrefix(m.ID, runID+":") {
					continue
				}
				if _, err := d.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
					if key == stream {
						pipe.XAck(ctx, key, group, entry.ID)
					}
					pipe.XDel(ctx, key, entry.ID)
					return nil
				}); err != nil {
					return err
				}
			}
			cursor = "(" + entries[len(entries)-1].ID
		}
	}
	var cursor uint64
	for {
		values, next, err := d.client.ZScan(ctx, base+":delay", cursor, "*", 200).Result()
		if err != nil {
			return err
		}
		for i := 0; i < len(values); i += 2 {
			var m TaskMessage
			if err := json.Unmarshal([]byte(values[i]), &m); err != nil {
				return err
			}
			if m.TenantKey == tenant && m.SubscriberID == subscriber && strings.HasPrefix(m.ID, runID+":") {
				if err := d.client.ZRem(ctx, base+":delay", values[i]).Err(); err != nil {
					return err
				}
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	for _, id := range messageIDs {
		if !strings.HasPrefix(id, runID+":") {
			return fmt.Errorf("retirement task identity mismatch")
		}
		lease, fence := taskLeaseKeys(tenant, subscriber, id)
		if _, err := d.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			// 队列项已移除，撤销在途投递租约，阻止迟到的 NACK 重新排队。
			pipe.Del(ctx, lease)
			pipe.ExpireAt(ctx, fence, at)
			pipe.ExpireAt(ctx, taskDedupeKey(tenant, subscriber, id), at)
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
