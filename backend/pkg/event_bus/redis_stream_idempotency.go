package event_bus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrTaskIdempotencyConflict = errors.New("TASK_QUEUE_IDEMPOTENCY_CONFLICT")
var ErrTaskDeliveryStateInvalid = errors.New("TASK_QUEUE_DELIVERY_STATE_INVALID")

// enqueueWithIdempotency 同时约束消息编号和幂等键，并核对不可变信封。
// 新服务合同使用独立主体的 subscriber 命名空间，不重写历史去重记录。
func (d *RedisStreamTaskDriver) enqueueWithIdempotency(ctx context.Context, message TaskMessage, raw []byte) error {
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	idKey := d.dedupeKey(message.TenantKey, message.SubscriberID, "id:"+message.ID)
	keyKey := d.dedupeKey(message.TenantKey, message.SubscriberID, "key:"+message.IdempotencyKey)
	for attempt := 0; attempt < 5; attempt++ {
		err := d.client.Watch(ctx, func(tx *redis.Tx) error {
			idValue, idErr := tx.Get(ctx, idKey).Result()
			keyValue, keyErr := tx.Get(ctx, keyKey).Result()
			if idErr != nil && !errors.Is(idErr, redis.Nil) {
				return idErr
			}
			if keyErr != nil && !errors.Is(keyErr, redis.Nil) {
				return keyErr
			}
			if idErr == nil || keyErr == nil {
				if idErr == nil && idValue != fingerprint || keyErr == nil && keyValue != fingerprint {
					return ErrTaskIdempotencyConflict
				}
				if idErr != nil || keyErr != nil {
					return ErrTaskDeliveryStateInvalid
				}
				return nil
			}
			_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, idKey, fingerprint, d.dedupeTTL)
				pipe.Set(ctx, keyKey, fingerprint, d.dedupeTTL)
				if message.VisibleAt.After(time.Now()) {
					stream := d.streamKey(message.TenantKey, message.SubscriberID)
					pipe.ZAdd(ctx, stream[:len(stream)-len(":stream")]+":delay", redis.Z{Score: float64(message.VisibleAt.UnixMilli()), Member: string(raw)})
				} else {
					pipe.XAdd(ctx, &redis.XAddArgs{Stream: d.streamKey(message.TenantKey, message.SubscriberID), Values: map[string]any{"message": string(raw)}})
				}
				return nil
			})
			return err
		}, idKey, keyKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return err
	}
	return ErrLeaseBusy
}
