# AI 设置与主模型生效流程

PowerX 的「AI 设置」页面提供了统一的 Provider/模型管理；理解其落库与生效顺序，能帮你快速定位“为什么切换 provider 没有真正生效”的问题。

## 默认配置

1. **后端默认值**：若从未在后台保存任何配置，Agent 将使用 `backend/etc/config_example.yaml` 中 `ai.defaults` 段配置的 provider / model / endpoint。这是编译期的静态兜底，适合本地开发。
2. **环境隔离**：页面左侧的环境切换（默认 `default`）会把不同环境的数据保存在各自的 scope 下，后端通过 `env + tenant` 二元索引区分，同一个 provider 可以在不同环境配置不同的 key。

## 保存一次 = 落两个实体

点击页面的「保存」会触发 `AgentSettingService.SaveCredentialAndProfile`，落库逻辑如下：

| 实体 | 说明 |
| --- | --- |
| `ai_provider_credentials` | 记录 provider 的 `base_url`、`api_key` 等敏感字段。重复保存会自动加密并“保留旧的 __sealed”。 |
| `ai_model_profiles` | 记录当前模态的 provider/model 组合以及默认参数（温度、max_tokens 等）。 |

换句话说，你可以先把不同 provider 的凭据都保存好，作为候选。此时数据库里会有多条 profile/credential，但 Agent 仍然会沿用「当前激活配置」。

## 选择谁当主模型？

真正决定“系统主智能体”使用哪个 provider/model 的，是 `ai_route_policies` 中的默认路由（`SetActiveProfile` 会写入 Name=`__default` 的记录）：

1. 在页面切换到目标 provider/model。
2. （可选）点击「测试连接」保证连通性通过。
3. 点击「保存」。这一步不仅更新 profile/credential，还会把该 provider/model 写入默认路由，后端随即改用新的组合。

> ⚠️ **仅切换下拉框不生效**：如果你仅切换 provider 但没有点击「保存」，前端状态会改变，但数据库里的默认路由不会更新，Agent 仍然调用旧模型。

## 多 provider 场景

- 你可以为多个 provider 各自保存凭据、参数，但同一模态下始终只有一个“激活”组合。需要切换时，选中后再次点击「保存」，系统会覆盖默认路由。
- 若想恢复默认（或某个候选）配置，只需切到对应 provider，检查参数无误后点击「保存」即可。
- 删除/禁用某 provider 之前，建议先把默认路由切到其他 provider，避免出现“找不到激活 profile”。

## 常见问题排查

| 现象 | 可能原因 | 处理方式 |
| --- | --- | --- |
| 后端日志仍显示旧 provider | 切换后未点击「保存」；或保存失败 | 查看页面提示/浏览器 Network，确认 `…/settings/save` 返回成功 |
| Base URL 不随 provider 清空 | 老版前端会缓存，记得刷新页面或在「保存」前手动覆盖 | 当前版本已在 provider 切换时同步；若仍遇到，重载页面确认 |
| 环境切换后配置不对 | 环境 store 未刷新 | 先切换环境，下方状态加载完成后再进行操作 |

掌握上述流程后，可以放心地把多个 provider 的信息一次性录入，只需记得“切换谁 → 就立即保存”来更新默认主模型。 ***
