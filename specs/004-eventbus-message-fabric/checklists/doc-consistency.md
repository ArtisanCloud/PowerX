# Doc Consistency Checklist（WS/TaskBus）

> 用于防止 `004-eventbus-message-fabric` 与 `023-websocket-notify` 契约漂移。

## PR 检查项（建议复制到 PR 模板）

- [x] 本次变更是否涉及 WS/TaskBus 代码（topic、publish/register、envelope、鉴权、tenant/trace）？
- [x] 若涉及，是否同步更新 `specs/004-eventbus-message-fabric/spec.md`（主契约）？
- [x] 若涉及 WS 传输，是否同步更新 `specs/023-websocket-notify/spec.md` 与 `specs/023-websocket-notify/contracts/http-openapi.yaml`？
- [x] 是否补充/更新联调场景（`local+proxy=0/1`、`mode=taskbus/dual/fallback`）？
- [x] 若涉及 Queue Driver（redis/kafka/rabbitmq/nats），是否同步更新 `spec.md` 的 Driver/Fallback 规则与 `quickstart.md` 的切换验证步骤？

## CI 一致性检查最低规则

1. **Topic 列表一致性**
   - 检查 `004` 主契约中的 topic 规则与 `023` 示例 topic 不冲突。
2. **接口路径一致性**
   - `023` 契约中必须包含：
     - `/api/v1/internal/ws-bus/register`
     - `/api/v1/internal/ws-bus/publish`
3. **Envelope 字段一致性**
   - `023` 的 `WSBusEnvelope` 必填字段至少包含：
     - `topic`
     - `type`
     - `payload`
     - `ts`
     - `trace_id`
4. **受保护 topic 授权一致性**
   - 文档必须明确：受保护 topic 不因动态注册绕过 RBAC（与实现一致）。
5. **Queue Driver 一致性**
   - `spec.md` 必须声明：数据库轮询仅为 fallback，不是默认主路径。
   - `quickstart.md` 必须包含 driver 切换（redis/kafka/rabbitmq/nats）与降级日志验证。
   - 若实现输出降级日志，文档中必须出现标准字段：`driver`、`tenant`、`reason`。

## 建议脚本化（后续）

- 在 CI 增加文档检查脚本（例如 `scripts/specs/check_ws_taskbus_contracts.sh`），至少执行：
  - `rg` 校验关键路径与字段；
  - 对比 `004` 与 `023` 的关键关键字集合；
  - 检查缺失项时返回非 0 退出码。
