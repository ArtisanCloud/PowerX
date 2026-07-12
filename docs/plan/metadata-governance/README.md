# Metadata Governance 开发计划

## 背景

PowerX 当前存在多个零散的元数据形态：

- 业务实体表内直接保存 `tags JSONB`。
- 局部模块自行维护字典或分类枚举。
- 插件在 local/delegated 模式下可能各自维护一套默认值。

这些做法会导致同一个业务概念在底座、插件、页面、Agent 能力里出现多份定义。`metadata-governance` 的目标是把“数据字典、分类体系、标签”沉淀为底座治理能力，插件和业务模块通过统一 API / capability / framework client 使用。

## 范围

本 feature 包含三类对象，但不把它们混成一张泛化表：

- 数据字典：字段枚举，例如客户来源、客户等级、任务优先级。
- 分类体系：层级归类，例如知识库目录、内容类目、商品分类。
- 标签：面向实体的自由或受控标注，例如客户标签、文件标签、Agent 标签。

不在本阶段实现：

- 复杂 MDM 主数据合并。
- 全文检索标签推荐模型。
- 历史业务表的自动回填迁移。
- 插件私有元数据的兼容读取。

## 设计原则

- 底座是 canonical source，插件不得各自发明同名业务字典。
- 所有业务对象表必须有 `uuid`；关系表可以没有 `uuid`，但外键必须引用对象 `uuid`。
- 用户可见名称必须走 i18n，不把 code、uuid、技术 ID 当主展示文案。
- local/delegated 行为必须显式：local 使用插件本地开发数据，delegated 通过 PowerX gateway 调用底座契约。
- 不提供隐式 fallback。命名空间、字典项、分类节点或标签不存在时返回明确错误。
- 迁移和运行时启动分离，不在后台启动时自动建表。

## 文档结构

- [mechanisms.md](./mechanisms.md)：机制、数据模型、API、capability、插件 framework。
- [pages.md](./pages.md)：管理端页面、交互、i18n 和权限入口。
- [rules.md](./rules.md)：命名、租户隔离、数据约束、发布和验收规则。

## 交付批次

### 第一批：底座契约和模型

- 新增 metadata governance 后端模型和 migration。
- 提供字典、分类、标签三组 admin API。
- 声明 platform capabilities，并纳入 `make capability-check`。
- 新增 service/repository 测试。

### 第二批：管理页面

- 设置区新增“元数据治理”入口。
- 页面分为“数据字典 / 分类体系 / 标签”三个 tab。
- 支持按 namespace、模块、状态筛选。
- 所有用户可见文案进入 locale 文件。

### 第三批：插件 framework 对齐

- framework 提供 MetadataClient。
- 插件侧通过 gateway capability 或 REST binding 访问底座。
- local 模式只用于插件独立开发，seed 与底座 canonical seed 同源。

### 第四批：业务接入

- 客户、知识库、Agent、媒体等模块逐步从私有 tags/dictionary 迁移到治理接口。
- 每个接入模块必须提交数据映射和迁移说明。
