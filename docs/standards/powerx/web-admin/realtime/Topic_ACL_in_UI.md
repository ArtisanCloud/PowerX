# Topic ACL 前端策略（ws-bus）

> 本文描述当前 ws-bus 场景下前端如何判断与呈现 topic 权限状态。

## 1. 当前事实

1. 当前无稳定的“Topic ACL 枚举接口”（如 `GET /agents/{id}/topics` 仅可作为规划示例）。
2. 前端实际判定来源：
- grant 响应（是否命中 `data.topics`）
- ws subscribe 回执（是否 `permission_denied/topic not allowed`）

## 2. 前端判定规则

1. grant 命中 + subscribe 成功
- UI：正常展示实时进度。

2. grant 仅 fallback（`topics: []`）
- UI：提示“授权未命中 topic 定义，可能无法订阅”。

3. subscribe 返回 `topic not allowed`
- UI：显示只读/告警态，禁止继续等待实时进度。
- 文案建议：`WS 未通过主题授权，请联系管理员检查 Topic Registry/ACL。`

## 3. 推荐交互时序

1. 触发同步前先 grant。
2. grant 命中后发送 subscribe。
3. 收到首个进度事件再进入“实时中”状态。
4. 若超时无事件，提示排障入口（grant 响应 + ws 错误详情）。

## 4. QA 检查清单

1. grant 命中时：前端可收到进度并刷新 UI。
2. grant fallback 时：前端能给出明确风险提示。
3. subscribe 被拒绝时：前端显示 `topic not allowed` 语义，不静默失败。
4. 切换租户后，topic 与 tenant 一致性校验生效。

## 5. 后续规划（非当前能力）

1. 提供 Topic ACL 枚举接口给前端一次性拉取。
2. 在连接前完成权限预判，减少 subscribe 失败重试。
3. 与权限申请/通知中心联动。
