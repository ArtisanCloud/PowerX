# Capability Registry & Router Benchmark Guide

为验证 Phase 6 的性能目标（Registry 查询 99% ≤150ms，Router 自动降级 500ms 内完成），项目新增了基准测试与辅助脚本。

## 运行方式

```bash
# 基准测试（以 go test benchmark 为基础）
scripts/perf/capability_registry_bench.sh

# 或手动执行
go test -run=^$ -bench=. -benchmem ./internal/tests/perf/capability_registry
```

脚本会自动将输出写入 `reports/capability_registry_bench.txt`，包含以下基准：

| Benchmark | 说明 |
|-----------|------|
| `BenchmarkRouterInvokePrimary` | 正常情况下的路由调用延迟 |
| `BenchmarkRouterFallback`      | 所有适配器失效时的 fallback 延迟 |
| `BenchmarkRegistryGetLatest`   | Registry 快照读取延迟 |

## 验收标准

- `BenchmarkRouterInvokePrimary` 平均耗时应稳定在 150ms 以内。
- `BenchmarkRouterFallback` 侧重观测最大耗时；脚本会输出 `max latency` 字段，需保持低于 500ms。
- `BenchmarkRegistryGetLatest` 用于检测最新快照查询是否满足 150ms 目标。

> 实际表现会受到运行环境、Postgres/Redis 配置以及 CPU 负载影响。建议在预发或性能环境执行，并结合 Prometheus/日志指标观察。

## 关联脚本

- `scripts/demo/capability_registry_route.sh`：演示注册 → 路由 → discovery 缓存流程，可在基准前/后快速验证功能链路。
- `scripts/perf/capability_registry_bench.sh`：收集基准数据并写入 `reports/` 目录，便于提交流程或追踪历史趋势。

