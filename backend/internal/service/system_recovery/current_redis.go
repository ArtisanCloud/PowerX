package system_recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/redis/go-redis/v9"
)

type currentRedisEntry struct {
	Key       string `json:"key"`
	Dump      []byte `json:"dump"`
	ExpiresAt int64  `json:"expires_at"`
}

type currentRedisEndpoint struct {
	role    string
	options redis.Options
}
type currentRedisImage struct {
	Version   int                            `json:"version"`
	Databases map[string][]currentRedisEntry `json:"databases"`
}

func currentRedisEndpoints(cfg *config.Config) ([]currentRedisEndpoint, error) {
	if cfg.Queue.Driver != "" && cfg.Queue.Driver != "local" && cfg.Queue.Driver != "redis" {
		return nil, errors.New("当前恢复尚未接入外部持久队列")
	}
	if cfg.Event.Bus.Type != "" && cfg.Event.Bus.Type != "local" && cfg.Event.Bus.Type != "redis" {
		return nil, errors.New("当前恢复尚未接入外部事件总线")
	}
	all := []currentRedisEndpoint{}
	// 缓存按安装与代次隔离并重建；不能因共享缓存库还有连接就阻止恢复，
	// 更不能把它与持久队列一起 FLUSHDB。这里只处理需恢复的队列／事件库。
	if cfg.Queue.Driver == "redis" {
		all = append(all, currentRedisEndpoint{"queue", redis.Options{Addr: cfg.Queue.Redis.Addr, Password: cfg.Queue.Redis.Password, DB: cfg.Queue.Redis.DB, PoolSize: 1}})
	}
	if cfg.Event.Bus.Type == "redis" {
		all = append(all, currentRedisEndpoint{"events", redis.Options{Addr: cfg.Event.Bus.RedisAddr, Password: cfg.Event.Bus.RedisPassword, DB: cfg.Event.Bus.RedisDB, PoolSize: 1}})
	}
	result := []currentRedisEndpoint{}
	seen := map[string]string{}
	for _, endpoint := range all {
		key := fmt.Sprintf("%s/%d", endpoint.options.Addr, endpoint.options.DB)
		if password, ok := seen[key]; ok {
			if password != endpoint.options.Password {
				return nil, errors.New("同一 Redis 逻辑库的凭据配置不一致")
			}
			continue
		}
		seen[key] = endpoint.options.Password
		result = append(result, endpoint)
	}
	return result, nil
}
func checkCurrentRedisClient(ctx context.Context, c *redis.Client, db int) error {
	id, err := c.ClientID(ctx).Result()
	if err != nil {
		return errors.New("无法读取 Redis 维护连接")
	}
	list, err := c.ClientList(ctx).Result()
	if err != nil {
		return errors.New("Redis 不允许核对维护连接")
	}
	for _, line := range strings.Split(list, "\n") {
		fields := map[string]string{}
		for _, field := range strings.Fields(line) {
			key, value, ok := strings.Cut(field, "=")
			if ok {
				fields[key] = value
			}
		}
		if fields["db"] == strconv.Itoa(db) && fields["id"] != strconv.FormatInt(id, 10) {
			return errors.New("仍有外部连接，未清理或覆盖数据；持久任务库需要独立于其他应用")
		}
	}
	return nil
}
func checkCurrentRedis(ctx context.Context, cfg *config.Config) error {
	endpoints, err := currentRedisEndpoints(cfg)
	if err != nil {
		return err
	}
	for _, endpoint := range endpoints {
		c := redis.NewClient(&endpoint.options)
		err := checkCurrentRedisClient(ctx, c, endpoint.options.DB)
		_ = c.Close()
		if err != nil {
			return fmt.Errorf("Redis %s 任务库（DB %d）检查失败: %w", endpoint.role, endpoint.options.DB, err)
		}
	}
	return nil
}
func captureCurrentRedis(ctx context.Context, cfg *config.Config, file string) error {
	endpoints, err := currentRedisEndpoints(cfg)
	if err != nil {
		return err
	}
	image := currentRedisImage{Version: 1, Databases: map[string][]currentRedisEntry{}}
	for _, endpoint := range endpoints {
		c := redis.NewClient(&endpoint.options)
		entries, err := captureCurrentRedisClient(ctx, c, endpoint.options.DB)
		_ = c.Close()
		if err != nil {
			return err
		}
		image.Databases[endpoint.role] = entries
	}
	data, err := json.Marshal(image)
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0600)
}
func captureCurrentRedisClient(ctx context.Context, c *redis.Client, db int) ([]currentRedisEntry, error) {
	if err := checkCurrentRedisClient(ctx, c, db); err != nil {
		return nil, err
	}
	// 单次 Lua 原子读取，避免逐键采集期间得到相互不一致的队列状态。
	value, err := c.Eval(ctx, `local keys=redis.call('KEYS','*');if #keys>100000 then return redis.error_reply('too many keys') end;local out={};local size=0;for _,k in ipairs(keys) do local d=redis.call('DUMP',k);size=size+#d;if size>67108864 then return redis.error_reply('too much data') end;table.insert(out,{k,d,redis.call('PTTL',k)}) end;return out`, nil).Slice()
	if err != nil {
		return nil, errors.New("Redis 状态过大或无法一致采集，未覆盖当前实例")
	}
	entries := []currentRedisEntry{}
	for _, item := range value {
		row, ok := item.([]any)
		if !ok || len(row) != 3 {
			return nil, errors.New("Redis 维护快照格式无效")
		}
		key, ok := row[0].(string)
		if !ok {
			return nil, errors.New("Redis key 格式无效")
		}
		dump, ok := row[1].(string)
		if !ok {
			return nil, errors.New("Redis dump 格式无效")
		}
		ttl, ok := row[2].(int64)
		if !ok {
			return nil, errors.New("Redis TTL 格式无效")
		}
		expires := int64(0)
		if ttl >= 0 {
			expires = time.Now().UnixMilli() + ttl
		}
		entries = append(entries, currentRedisEntry{key, []byte(dump), expires})
	}
	return entries, nil
}
func restoreCurrentRedis(ctx context.Context, cfg *config.Config, data []byte) error {
	endpoints, err := currentRedisEndpoints(cfg)
	if err != nil {
		return err
	}
	image := currentRedisImage{Version: 1, Databases: map[string][]currentRedisEntry{}}
	if len(data) > 0 {
		if len(data) > 96*1024*1024 || json.Unmarshal(data, &image) != nil || image.Version != 1 {
			return errors.New("Redis 保护快照无效")
		}
	}
	if err = checkCurrentRedis(ctx, cfg); err != nil {
		return err
	}
	for _, endpoint := range endpoints {
		entries, ok := image.Databases[endpoint.role]
		if len(data) > 0 && !ok {
			return errors.New("Redis 保护快照与当前已配置逻辑库不一致")
		}
		c := redis.NewClient(&endpoint.options)
		err := restoreCurrentRedisClient(ctx, c, endpoint.options.DB, entries)
		_ = c.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
func restoreCurrentRedisClient(ctx context.Context, c *redis.Client, db int, entries []currentRedisEntry) error {
	if err := checkCurrentRedisClient(ctx, c, db); err != nil {
		return err
	}
	args := []any{}
	for _, entry := range entries {
		ttl := int64(0)
		if entry.ExpiresAt > 0 {
			ttl = entry.ExpiresAt - time.Now().UnixMilli()
			if ttl <= 0 {
				continue
			}
		}
		args = append(args, entry.Key, ttl, string(entry.Dump))
	}
	_, err := c.Eval(ctx, `redis.call('FLUSHDB');for i=1,#ARGV,3 do redis.call('RESTORE',ARGV[i],ARGV[i+1],ARGV[i+2],'REPLACE') end;return 1`, nil, args...).Result()
	if err != nil {
		return errors.New("Redis 恢复失败，已保留维护现场")
	}
	return nil
}
