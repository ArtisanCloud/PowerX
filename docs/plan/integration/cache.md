# Cache（统一缓存）

## 目标
- 统一缓存抽象，服务端与插件可复用同一套缓存接口。
- 默认使用 Redis，可扩展为多级缓存（本地 + Redis）。
- 明确租户隔离、过期策略与观测指标。

## 统一抽象
- **CacheStore**：get/set/delete/ttl/incr。
- **CacheKey**：命名约束（tenant + domain + key）。
- **CachePolicy**：过期策略、失效策略、热点保护。

## 初始化流程（建议）
1) 读取配置：`cache` 块（宿主注入或插件本地）。
2) 解析驱动与连接参数（redis_url 优先）。
3) 注册 Provider（内置 + 自定义）。
4) 实例化 CacheStore 并注入到业务依赖。
5) 设置默认 TTL、prefix 与降级策略。

## 驱动与选择规则
### 支持驱动
- `redis`：默认生产驱动（共享缓存）。
- `memory`：本地进程内缓存（开发/单体场景）。
- `noop`：空实现（禁用缓存）。
- `custom`：自定义驱动（通过 Provider 注册）。

### 选择优先级
1) `cache.driver`（config.yaml / host-values.yaml）
2) 环境变量 `POWERX_CACHE_DRIVER`
3) 默认 `redis`（宿主模式） / `memory`（本地开发）

## 驱动配置规范
### Redis
```yaml
cache:
  driver: redis
  redis_url: "redis://:password@127.0.0.1:6379/3"
  # 或使用拆分字段
  host: 127.0.0.1
  port: 6379
  password: ""
  db: 3
  prefix: "powerx:{tenant_uuid}"
  default_ttl: 10m
  dial_timeout: 3s
  read_timeout: 2s
  write_timeout: 2s
```

### Memory
```yaml
cache:
  driver: memory
  max_entries: 50000
  default_ttl: 5m
  cleanup_interval: 1m
```

### Noop
```yaml
cache:
  driver: noop
```

## 驱动扩展（Provider）
```
type CacheProvider interface {
  Name() string
  New(config CacheConfig) (CacheStore, error)
}
```
- 插件/宿主在启动时注册 Provider。
- `cache.driver` 与 Provider.Name 匹配即可切换。

## 读写路径与降级
- 读路径：优先走 L1（若启用）→ L2（Redis）。
- 写路径：写 L2，再异步刷新 L1。
- 驱动不可用：记录告警并降级为 `noop`（可配置是否允许）。

## Key 规范
- `cache.prefix = "powerx:{tenant_uuid}"`
- 业务侧 key 示例：`kb:chunks:{chunk_id}` → 实际 key：`powerx:tenant123:kb:chunks:abc`

## TTL 与热点保护
- 默认 TTL 由 `cache.default_ttl` 控制（建议 5m~30m）。
- 热点 Key 加互斥锁（mutex key）避免缓存击穿。

## 代码接口（建议）
```
type CacheStore interface {
  Get(ctx, key string) (value []byte, ok bool)
  Set(ctx, key string, value []byte, ttl time.Duration) error
  Delete(ctx, key string) error
  TTL(ctx, key string) (time.Duration, error)
  Incr(ctx, key string, delta int64) (int64, error)
}
```

## 宿主模式配置（PowerX 注入）
- 插件不在 `plugin.yaml` 配置缓存。
- 宿主在启动插件时注入 `host-values.yaml` / `config.yaml` 的 `cache` 块。
- 插件侧统一读取 `cache` 配置（结构与 PowerX Core 相同）。
- Standalone 模式由插件自身 `backend/etc/config.yaml` 提供 `cache` 块。

示例（注入到插件的 config.yaml）：
```yaml
cache:
  driver: redis
  host: localhost
  port: 6379
  password: ""
  db: 0
  prefix: "powerx:{tenant_uuid}"
```

## 插件使用示例（伪代码）
```
cache.Set(ctx, "kb:chunks:123", bytes, 10*time.Minute)
val, ok := cache.Get(ctx, "kb:chunks:123")
```

## 测试与验证
- Memory 驱动用于单元测试，默认不依赖 Redis。
- 关键路径加 metrics：hit/miss/latency。
- 故障注入：模拟 Redis 不可用，验证降级策略。
