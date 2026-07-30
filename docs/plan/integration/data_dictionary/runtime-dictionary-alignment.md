# 系统数据字典对齐实现（PowerX 底座口径）

## 1. 目标

把来源、渠道、类型等“可枚举配置”沉淀为底座通用能力，供插件统一复用。

目标效果：

- 插件安装后即可使用（有默认 seed）。
- `POWERX_PROVIDER_MODE=local` 与 `POWERX_PROVIDER_MODE=delegated` 行为一致。
- 避免每个插件维护私有字典造成分叉。

## 2. 统一接口（Runtime）

建议基线接口：

- `GET /api/v1/admin/runtime/dictionaries`
- `POST /api/v1/admin/runtime/dictionaries`
- `PATCH /api/v1/admin/runtime/dictionaries/:item_id`
- `DELETE /api/v1/admin/runtime/dictionaries/:item_id?namespace=...`

字段基线：

- `item_id`
- `namespace`
- `code`
- `label`
- `sort`
- `enabled`

## 3. 命名空间策略

- namespace 支持动态扩展，不做固定白名单。
- 可提供推荐常量（如 `scrm.lead.traffic_platform`、`scrm.lead.traffic_source`），但不限制新增。

## 4. 模式分流规范

- `POWERX_PROVIDER_MODE=local`：读取插件本地字典存储。
- `POWERX_PROVIDER_MODE=delegated`：插件通过 framework delegated provider 调用 PowerX Core 字典能力。
- `POWERX_PROXY` 只控制是否连接 PowerX 宿主运行时链路，不参与字典 provider 分流。

原则：业务层只调一个 `DictionaryService`，模式分流在服务内部完成。

## 5. Seed 对齐策略（关键）

必须采用“双端同源 seed”：

1. 定义一份 canonical seed 清单（建议 `namespace + code + label + sort + enabled`）。
2. PowerX 底座 seed 使用该清单。
3. 插件本地 seed 使用同一清单。
4. 写入方式使用 upsert（按 `namespace+code` 幂等）。

这样才能保证：

- standalone 安装后有默认选项；
- delegated 场景读取宿主字典也有默认选项；
- 两边枚举一致。

## 6. RBAC 建议

字典接口纳入 `runtime.ops`：

- `GET` -> `runtime.ops:read`
- `POST/PATCH/DELETE` -> `runtime.ops:manage`

## 7. 前端基线（管理端）

统一入口：`/admin/iam/dictionaries`

建议交互：

1. 按 namespace 自动分组
2. 每组支持折叠/展开
3. 每组独立分页
4. 通用 CRUD
5. 深浅主题可读性达标

## 8. 与 SCRM 当前实现对齐点

- SCRM 已使用系统字典接口承载线索来源配置。
- 旧私有入口应跳转系统字典页，避免双入口。
- SCRM 仅作为业务消费方，不再维护私有字典体系。

## 9. 验收清单

1. 安装后无需手工配置即可看到默认字典项（seed 生效）。
2. local/delegated 下字典操作与展示一致。
3. namespace 动态新增可用。
4. 分组折叠、分页、CRUD 可用。
5. 不出现“双字典源”。
