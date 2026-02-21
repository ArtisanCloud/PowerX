# Testing / 回归测试（async_runtime）

> 状态：已实现（基础脚本）  
> 平台入口：`docs/guides/async_runtime/README.md`

## 1. 已有测试入口

1. Event Fabric 联调：`scripts/event_fabric/integration_playbook.sh`
2. WebSocket 联调：`scripts/websocket/integration_playbook.sh`
3. Cron 联调：`scripts/cron/integration_playbook.sh`
4. 无服务层快速回归：`scripts/ci/event_fabric_layer1.sh`

## 2. 推荐顺序

1. 先跑 `scripts/ci/event_fabric_layer1.sh`（不依赖服务）
2. 再跑 `scripts/event_fabric/integration_playbook.sh --with-write`
3. 再跑 `scripts/websocket/integration_playbook.sh --with-write`
4. 最后跑 `scripts/cron/integration_playbook.sh --with-write`
5. 最后做管理台手工联调验收

## 3. 待补齐项（占位）

1. Logs/Trace 字段完整性校验脚本
2. 失败注入（chaos）回归脚本
