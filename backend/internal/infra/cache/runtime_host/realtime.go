package runtime_host

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/redisnamespace"
	"github.com/redis/go-redis/v9"
)

// Realtime 历史是有界恢复缓冲，不承载业务任务、ACK 或业务重投。
const RealtimeRetention = time.Hour
const RealtimeCapacity = 2048

var ErrRealtimeCursorExpired = errors.New("REALTIME_CURSOR_EXPIRED")

type RealtimeEntry struct {
	Cursor string
	Value  []byte
}

func (s *Store) realtimeKey(tenant, subject, namespace string) string {
	return redisnamespace.Key(s.namespace, strings.Replace(Key(tenant, subject, namespace, "events"), "cache:v1:", "realtime:v1:", 1))
}

var realtimeAppend = redis.NewScript(`
local id=redis.call('XADD',KEYS[1],'MAXLEN',2048,'*','event',ARGV[1])
redis.call('EXPIRE',KEYS[1],3600)
return id
`)

func (s *Store) AppendRealtime(ctx context.Context, tenant, subject, namespace string, value []byte) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("REALTIME_UNAVAILABLE")
	}
	if len(value) > 128<<10 || !json.Valid(value) {
		return "", errors.New("REALTIME_PAYLOAD_INVALID")
	}
	return realtimeAppend.Run(ctx, s.client, []string{s.realtimeKey(tenant, subject, namespace)}, value).Text()
}
func (s *Store) RealtimeCursor(ctx context.Context, tenant, subject, namespace string) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("REALTIME_UNAVAILABLE")
	}
	rows, err := s.client.XRevRangeN(ctx, s.realtimeKey(tenant, subject, namespace), "+", "-", 1).Result()
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "0-0", nil
	}
	return rows[0].ID, nil
}

// 每个副本独立持有游标；不使用消费组抢占其他副本的浏览器事件。
func (s *Store) ReadRealtime(ctx context.Context, tenant, subject, namespace, cursor string) ([]RealtimeEntry, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("REALTIME_UNAVAILABLE")
	}
	key := s.realtimeKey(tenant, subject, namespace)
	head, err := s.client.XRangeN(ctx, key, "-", "+", 1).Result()
	if err != nil {
		return nil, err
	}
	if cursor != "0-0" && (len(head) == 0 || compareStreamID(cursor, head[0].ID) < 0) {
		return nil, ErrRealtimeCursorExpired
	}
	rows, err := s.client.XRead(ctx, &redis.XReadArgs{Streams: []string{key, cursor}, Count: 64, Block: time.Second}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// XREAD 与裁剪可能并发；交付前复核头部，不能隐式跳过丢失区间。
	if cursor != "0-0" {
		latest, e := s.client.XRangeN(ctx, key, "-", "+", 1).Result()
		if e != nil {
			return nil, e
		}
		if len(latest) == 0 || compareStreamID(cursor, latest[0].ID) < 0 {
			return nil, ErrRealtimeCursorExpired
		}
	}
	out := []RealtimeEntry{}
	for _, stream := range rows {
		for _, entry := range stream.Messages {
			value, ok := entry.Values["event"].(string)
			if !ok {
				return nil, errors.New("REALTIME_ENTRY_INVALID")
			}
			out = append(out, RealtimeEntry{entry.ID, []byte(value)})
		}
	}
	return out, nil
}
func compareStreamID(a, b string) int {
	aa, bb := strings.SplitN(a, "-", 2), strings.SplitN(b, "-", 2)
	for i := 0; i < 2; i++ {
		if len(aa[i]) < len(bb[i]) {
			return -1
		}
		if len(aa[i]) > len(bb[i]) {
			return 1
		}
		if aa[i] < bb[i] {
			return -1
		}
		if aa[i] > bb[i] {
			return 1
		}
	}
	return 0
}
