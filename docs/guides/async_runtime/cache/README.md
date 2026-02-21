# Cache 管理（async_runtime）

> 状态：已实现（基础能力）  
> 平台入口：`docs/guides/async_runtime/README.md`

## 1. 范围

1. Redis 运行态队列缓存（TaskDriver）
2. Topic/ACL 查询缓存（服务侧）
3. 缓存失效与一致性排查

## 2. 已实现能力

1. Task 队列运行态基于 Redis（`q/d/p/i` 四类键）
2. 运行态统计通过 `task-queue/stats` 读取 Redis 聚合
3. Topic/ACL 查询链路存在缓存层（用于降低 DB 压力）

## 3. 运维检查

1. 先看队列状态：`GET /admin/event-fabric/task-queue/stats`
2. 再看分片历史：`GET /admin/event-fabric/task-queue/messages`
3. 排查缓存错觉时，以历史账本和业务结果双重确认

## 4. 后续补齐项（占位）

1. 统一缓存键字典（按模块列全）
2. TTL 与主动失效策略清单
3. 缓存预热与容量基线

