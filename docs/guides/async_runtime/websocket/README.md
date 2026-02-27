# WebSocket 子系统说明（async_runtime）

> 状态：已实现（基础能力）  
> 平台级入口：`docs/guides/async_runtime/README.md`
> 规格来源：`specs/023-websocket-notify/spec.md`

## 1. 你先看哪份

1. 快速联调（含 `wscat`/Host/Standalone/Proxy）：`docs/guides/async_runtime/websocket/debug_playbook.md`
2. 机制关联（Topic/Task 语义）：`docs/guides/async_runtime/task/mechanism.md`
3. 平台总览：`docs/guides/async_runtime/README.md`
4. 脚本联调（HTTP + Queue 校验）：`scripts/websocket/integration_playbook.sh`

## 2. 范围

1. 连接与鉴权（`/api/ws`）
2. 订阅语义（Topic + ACL）
3. 总线复用信道（单连接多 Topic）
4. 推送包结构（welcome/ack/error/event）
5. Host/Standalone/Proxy 三模式联调

## 3. 核心约定（与 `023` 对齐）

1. 单页面只维持一条 WS 连接（复用信道）。
2. 通过 `subscribe/unsubscribe` 在同一连接上管理多个 topic。
3. 未授权 topic 必须拒绝订阅（返回 `error`）。
4. 连接断开后允许回退机制，恢复后继续使用同一协议。

## 4. 约束

1. WebSocket 只负责实时分发，不承担任务执行。
2. 页面实时状态以 WS 推送为主，禁止前端轮询替代。
