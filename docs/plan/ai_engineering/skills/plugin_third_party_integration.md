# Skills 插件侧与第三方接入设计

本文描述插件开发者与第三方如何接入 Skill 能力。

## 1. 目标

1. 插件可发布 Skill 并绑定 capability。
2. 第三方可在受控流程中导入 Skill。
3. 既支持独立调用 Skill，也支持 Agent + Skill 组合调用。

## 2. 插件接入流程

1. 插件准备 Skill Bundle 与 `SKILL.md`。
2. 调用 Admin 导入接口注册 Skill。
3. 绑定 capability 与 tool grants。
4. 发布后由租户通过统一入口调用。

## 3. 第三方接入流程

1. 提交来源信息（仓库、版本、checksum、签名）。
2. 上传 Skill Bundle（平台不做远程仓库在线拉取）。
3. 平台校验资产与元数据。
4. 通过后进入 `draft` 状态。
5. 管理员审核发布到 `published`。

## 4. 开放模式

### 4.1 独立 Skill 模式

租户直接调用：

- `POST /api/v1/tenant/skills/invoke`

### 4.2 Agent + Skill 模式

Agent 在规划中引用 skill 节点，由 SkillRunner 执行。

### 4.3 统一 capability 模式

通过：

- `/api/v1/tenant/invocations`
- `preferred_protocol=skill`

## 5. 治理要求

1. 插件卸载前需处理 Skill 绑定关系。
2. 第三方 Skill 升级必须记录来源变更。
3. 不允许“无版本覆盖式更新”。

## 6. 最小对接清单

1. 提供 `SKILL.md`
2. 提供 bundle uri
3. 提供 checksum
4. 声明 entrypoints
5. 声明权限副作用
