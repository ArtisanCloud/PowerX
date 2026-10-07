package runtime_host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Entry struct {
	Found     bool
	Value     []byte
	ExpiresAt time.Time
}
type Store struct{ client redis.UniversalClient }

func NewStore(client redis.UniversalClient) *Store {
	if client != nil && reflect.ValueOf(client).Kind() == reflect.Ptr && reflect.ValueOf(client).IsNil() {
		client = nil
	}
	return &Store{client: client}
}
func Key(tenant, subject, namespace, key string) string {
	raw, _ := json.Marshal([]string{tenant, subject, namespace, key})
	digest := sha256.Sum256(raw)
	return "powerx:runtime-host:cache:v1:" + hex.EncodeToString(digest[:])
}

var setScript = redis.NewScript(`
local now = redis.call('TIME')
local expiry = now[1]*1000 + math.floor(now[2]/1000) + tonumber(ARGV[2])
redis.call('SET', KEYS[1], string.format('%.0f', expiry)..'\n'..ARGV[1], 'PX', ARGV[2])
return 1
`)
var getScript = redis.NewScript(`
local value = redis.call('GET', KEYS[1])
if not value then return false end
if redis.call('PTTL', KEYS[1]) < 0 then return redis.error_reply('runtime_host.invalid_expiry') end
return value
`)

func (s *Store) Get(ctx context.Context, key string) (*Entry, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("runtime_host.redis_unavailable")
	}
	raw, err := getScript.Run(ctx, s.client, []string{key}).Text()
	if errors.Is(err, redis.Nil) {
		return &Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	expiry, value, ok := strings.Cut(raw, "\n")
	ms, e := strconv.ParseInt(expiry, 10, 64)
	if !ok || e != nil || ms <= 0 {
		return nil, errors.New("runtime_host.invalid_entry")
	}
	return &Entry{Found: true, Value: []byte(value), ExpiresAt: time.UnixMilli(ms).UTC()}, nil
}
func (s *Store) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return errors.New("runtime_host.redis_unavailable")
	}
	return setScript.Run(ctx, s.client, []string{key}, value, ttl.Milliseconds()).Err()
}
func (s *Store) Delete(ctx context.Context, key string) error {
	if s == nil || s.client == nil {
		return errors.New("runtime_host.redis_unavailable")
	}
	return s.client.Del(ctx, key).Err()
}
