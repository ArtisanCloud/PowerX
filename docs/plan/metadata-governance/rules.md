# Metadata Governance 规则

## 命名规则

namespace 使用反向域名或模块前缀：

- `corex.customer.level`
- `corex.customer.source`
- `corex.knowledge.category`
- `plugin.<plugin_id>.<domain>.<name>`

code 使用小写 snake case：

- `vip`
- `new_customer`
- `high_priority`

resource type 使用稳定业务对象名：

- `customer.account`
- `knowledge.space`
- `agent.profile`
- `media.asset`

不允许：

- 使用 UUID 作为 namespace 或 code。
- 使用用户可见文案作为 code。
- 从自由文本里解析 namespace、code、resource type。
- 同一业务概念在多个 namespace 下重复定义，除非明确区分模块边界。

## i18n 规则

- 多语言字段统一使用 JSONB map。
- 首版必须包含 `zh-CN`。
- 管理页面编辑时允许补充 `en-US`、`ja-JP`、`ko-KR`。
- API 不得把 i18n map 里的任意语言值写入 `code` 或 `namespace`。
- 用户可见名称展示当前 locale 命中的 display 字段；未命中时必须显示明确缺失状态，不静默展示 code 作为主名称。

## 租户隔离

- 所有元数据对象必须带 `tenant_uuid`，平台全局 seed 也必须在租户初始化时实例化到租户维度。
- 查询和写入必须从鉴权上下文取租户，不接受客户端传入覆盖。
- root/support 场景跨租户访问必须走单独 root API，不复用 tenant admin API。

## UUID 规范

- `metadata_dictionary_namespaces`、`metadata_dictionary_items`、`metadata_taxonomies`、`metadata_taxonomy_nodes`、`metadata_tags` 必须有 `uuid`。
- `metadata_tag_bindings` 作为关系表可以没有 `uuid`。
- 所有关联字段使用对象 `uuid`，不使用自增 ID 作为外部映射。
- API 请求路径、payload、事件和审计里的业务对象引用使用 `uuid`。
- 多态引用不能使用自增 ID 或字符串业务名作为外部引用。

## 资源类型规则

- 每个可被标签绑定的业务对象必须先注册 `resource_type`。
- `resource_type` 创建后不可修改。
- 写入标签绑定前必须校验 `resource_type` 已启用。
- 写入标签绑定前必须校验业务对象存在且属于当前租户。
- 没有对象存在性校验器的 `resource_type` 只允许读取，不允许写入绑定。

## 状态规则

统一状态：

- `enabled`
- `disabled`
- `archived`

行为：

- `enabled` 可被新数据选择。
- `disabled` 不可被新数据选择，但历史引用继续可读。
- `archived` 仅管理员归档查看，不进入普通选择器。

## 删除规则

- 未被引用对象允许硬删除。
- 已被引用对象不允许硬删除，必须先迁移引用或改为 `disabled`。
- 合并标签必须显式选择 source tag 和 target tag，并记录审计。
- 删除接口不得静默忽略引用冲突。
- 删除判断以 `metadata_references` / `metadata_tag_bindings` 的实时查询为准，缓存的引用数只能用于展示。
- 删除冲突必须返回稳定错误码和引用摘要。

## 引用登记规则

- 字典项、分类节点被业务对象引用时，业务模块必须写入 `metadata_references`。
- 业务对象更新元数据字段时，必须同步替换引用登记。
- 业务对象删除或归档时，必须清理或标记引用登记。
- 引用登记失败时，业务写入必须失败，不允许只写业务字段不写引用。

## Seed 规则

- canonical seed 放在底座可审计目录。
- 插件 local seed 必须从同一份 canonical 定义生成或复制。
- delegated 模式不得读取插件私有 seed 作为 fallback。
- seed 只能通过显式命令或 tenant bootstrap 调用，不允许后台服务启动时自动执行。
- seed 文件必须经过 schema 校验，缺字段时 fail-fast。
- seed upsert key：
  - 字典 namespace：`tenant_uuid + namespace`
  - 字典项：`tenant_uuid + namespace + code`
  - 分类体系：`tenant_uuid + namespace`
  - 分类节点：`tenant_uuid + taxonomy_uuid + code`
  - 标签：`tenant_uuid + namespace + resource_type + code`
  - 资源类型：`tenant_uuid + resource_type`

## API 规则

- 管理端接口统一走 `/api/v1/admin/metadata/...`。
- 插件调用底座能力优先走 `/api/v1/tenant/invocations`。
- 对外开放给 web / mini-app / customer 的接口不得复用 admin 路径。
- 所有响应使用统一 envelope 和分页结构。
- 所有校验错误返回明确 code，不做兼容转换。

## Capability 规则

- 每个正式开放给插件或 Agent 的 metadata 能力必须进入 `backend/config/platform_capabilities`。
- 每个 capability 必须有 `permission_code`。
- capability_id 和 permission_code 必须在 spec 中显式映射。
- `agent_usable` 只能在能力确实适合 Agent 调用时设为 true。
- 高风险写操作必须标记 `risk_level`，并进入审计。
- 新增或修改 metadata API 后必须执行：

```bash
make capability-check
```

## 前端规则

- 选择器只显示名称，namespace/code 作为次级信息。
- 空状态必须说明是“没有数据”还是“未选择 namespace/resource type”。
- 表单固定宽度和高度约束，避免字段过长撑破布局。
- 所有用户可见文案必须走 i18n。
- 不允许在组件内硬编码业务字典项。

## 验收规则

后端：

- migration 可单独执行。
- service 单元测试覆盖 create/update/delete/list。
- repository 测试覆盖租户隔离和唯一约束。
- capability check 通过。

前端：

- 数据字典、分类体系、标签三个 tab 可独立加载和筛选。
- 无权限用户看不到管理按钮。
- 多语言切换后页面文案完整。
- 选择器不显示 UUID 作为主标签。

插件：

- delegated 模式能通过 gateway 读取底座 metadata。
- local 模式能用同源 seed 启动。
- 缺少 namespace 或 capability 授权时明确失败。

## 进入 `specs/029-metadata-governance` 前置条件

- 本 plan 中 i18n、资源类型、引用登记、seed、capability 映射规则已被 spec 接收。
- `spec.md` 必须按用户故事拆分 MVP 和后续批次。
- `data-model.md` 必须包含表字段、唯一索引、状态机和删除冲突规则。
- `contracts/http-openapi.yaml` 必须包含请求/响应 DTO 和错误码。
- `tasks.md` 必须先实现 migration/repository/service，再实现 HTTP/page/framework。
