# Event Fabric 文档入口（精简版）

> 平台级入口：`docs/guides/async_runtime/README.md`

## 1. 你只需要看这两份

1. 命名规范：`docs/guides/async_runtime/event_fabric/naming_convention.md`
2. 联调/运维/故障：`docs/guides/async_runtime/event_fabric/integration_playbook.md`

## 2. 跨域依赖（按需看）

Task 机制是跨域能力，统一放在：

1. `docs/guides/async_runtime/task/mechanism.md`

只有在你需要理解队列分片、生命周期、重试语义时再看，不作为 Event Fabric 子目录必读首项。

## 3. 兼容说明

已移除独立 `operations.md`，运维内容已并入：

1. `docs/guides/async_runtime/event_fabric/integration_playbook.md`
