package agent_run

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// ValidateProductionRedis checks every writable Redis node before the Agent
// runtime accepts work. An inaccessible CONFIG command is a failed gate: the
// caller must not infer safe persistence settings from a successful PING.
func ValidateProductionRedis(ctx context.Context, client redis.UniversalClient) error {
	if client == nil {
		return fmt.Errorf("agent run Redis client is required")
	}
	if cluster, ok := client.(*redis.ClusterClient); ok {
		checked := 0
		err := cluster.ForEachMaster(ctx, func(ctx context.Context, node *redis.Client) error {
			checked++
			if err := validateRedisNode(ctx, node); err != nil {
				return fmt.Errorf("agent run Redis master %s: %w", node.Options().Addr, err)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if checked == 0 {
			return fmt.Errorf("agent run Redis cluster has no writable master")
		}
		return nil
	}
	return validateRedisNode(ctx, client)
}

func validateRedisNode(ctx context.Context, client redis.UniversalClient) error {
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis ping: %w", err)
	}
	aof, err := client.ConfigGet(ctx, "appendonly").Result()
	if err != nil {
		return fmt.Errorf("read Redis appendonly: %w", err)
	}
	policy, err := client.ConfigGet(ctx, "maxmemory-policy").Result()
	if err != nil {
		return fmt.Errorf("read Redis maxmemory-policy: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(aof["appendonly"])) != "yes" {
		return fmt.Errorf("Redis appendonly must be yes")
	}
	if strings.ToLower(strings.TrimSpace(policy["maxmemory-policy"])) != "noeviction" {
		return fmt.Errorf("Redis maxmemory-policy must be noeviction")
	}
	info, err := client.Info(ctx, "persistence").Result()
	if err != nil {
		return fmt.Errorf("read Redis persistence health: %w", err)
	}
	return validatePersistenceHealth(info)
}

// 配置开启 AOF 还不足以证明持久化写入正常；缺失或错误的健康字段失败关闭。
func validatePersistenceHealth(info string) error {
	values := make(map[string]string)
	for _, line := range strings.Split(info, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok {
			values[key] = value
		}
	}
	if values["loading"] != "0" || values["aof_enabled"] != "1" || values["aof_last_write_status"] != "ok" || values["aof_last_bgrewrite_status"] != "ok" {
		return fmt.Errorf("Redis AOF persistence is not healthy")
	}
	return nil
}
