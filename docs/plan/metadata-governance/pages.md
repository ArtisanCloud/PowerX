# Metadata Governance 页面设计

## 信息架构

入口：`设置 > 元数据治理`

页面标题：`元数据治理`

页面说明：通过统一入口管理数据字典、分类体系和标签，供 PowerX 底座、插件、Agent 和业务页面复用。

一级 tabs：

- 数据字典
- 分类体系
- 标签
- 资源类型

## 页面状态

所有 tab 都必须显式区分以下状态：

- 未选择 namespace / taxonomy / resource type。
- 当前筛选条件下没有数据。
- 当前用户没有查看权限。
- 后端契约错误或 capability 未授权。
- 数据加载中。

不得把错误状态渲染成空列表。

## 数据字典 Tab

布局：

- 左侧：namespace 列表，可搜索，可按模块筛选。
- 右侧：当前 namespace 的字典项表格。

namespace 列表字段：

- 名称
- namespace
- 模块
- 状态
- 字典项数量

字典项表格字段：

- 名称
- code
- 状态
- 排序
- 引用数
- 更新时间

操作：

- 新建 namespace
- 编辑 namespace
- 新建字典项
- 编辑字典项
- 启用 / 停用字典项
- 删除未被引用的字典项

交互约束：

- code 创建后不可修改。
- namespace 创建后不可修改。
- 名称和描述以多语言表单编辑，至少填写 `zh-CN`。
- 被业务引用的字典项不能硬删除，只能停用。
- 字典项停用后，历史数据仍显示原名称，但新建/编辑表单不可选择。
- 删除前必须展示引用数；引用数大于 0 时删除按钮禁用或提交后返回冲突错误。

## 分类体系 Tab

布局：

- 左侧：taxonomy 列表，可搜索，可按模块筛选。
- 右侧：树形分类节点。

taxonomy 列表字段：

- 名称
- namespace
- 模块
- 最大层级
- 状态

节点字段：

- 名称
- code
- 状态
- 排序
- 子节点数量
- 引用数

操作：

- 新建 taxonomy
- 编辑 taxonomy
- 新建根节点
- 新建子节点
- 拖拽排序
- 移动节点
- 启用 / 停用节点
- 删除未被引用节点

交互约束：

- 超过 `max_depth` 时禁止创建或移动。
- 移动节点必须检测环。
- 停用父节点时必须提示会影响子节点可选状态。
- 不允许用自由文本解析层级路径；层级关系必须来自 `parent_uuid`。
- 拖拽移动必须展示明确保存状态；后端返回并发冲突时恢复到服务端最新树。
- 节点详情可展示 path 作为调试信息，但主显示仍使用名称。

## 标签 Tab

布局：

- 顶部：resource type 筛选器、namespace 筛选器、状态筛选器、搜索框。
- 主区：标签表格。
- 右侧抽屉：标签详情和绑定统计。

标签表格字段：

- 名称
- 颜色
- resource type
- namespace
- 状态
- 使用次数
- 更新时间

操作：

- 新建标签
- 编辑标签
- 启用 / 停用标签
- 合并标签
- 删除未绑定标签

交互约束：

- 标签名称可多语言，code 必须稳定。
- 标签颜色只用于辅助识别，不承载业务语义。
- 合并标签必须写入审计日志。
- 已绑定标签不能直接硬删除。
- 合并标签必须先选择 source tag 和 target tag，并显示受影响绑定数量。
- `resource type` 未选择时，不加载标签绑定统计。

## 资源类型 Tab

布局：

- 顶部：模块筛选器、状态筛选器、搜索框。
- 主区：资源类型表格。
- 右侧抽屉：资源类型详情、可绑定对象说明、校验器状态。

表格字段：

- 名称
- resource type
- 模块
- 状态
- 校验器状态
- 更新时间

操作：

- 注册资源类型。
- 编辑资源类型名称和描述。
- 启用 / 停用资源类型。

交互约束：

- `resource_type` 创建后不可修改。
- 已存在标签绑定的资源类型不能硬删除。
- 未配置对象存在性校验器的资源类型，页面必须提示“不可写入绑定”。

## 业务表单集成

业务页面不直接实现一套私有标签或字典管理。

推荐组件：

- `MetadataDictionarySelect`
- `MetadataTaxonomyTreeSelect`
- `MetadataTagPicker`

组件输入：

- `namespace`
- `resourceType`
- `modelValue`
- `disabled`
- `multiple`

组件行为：

- 只展示当前租户可用、状态为 enabled 的选项。
- 已停用但历史已选中的值以只读状态展示。
- 错误状态明确显示，不做空列表 fallback。

首批接入策略：

- 管理页面先交付治理后台，不强制迁移现有业务表单。
- 业务模块接入必须单独提交映射说明：字段名、namespace/resource type、历史值处理、引用登记策略。
- 客户、知识库、Agent、媒体作为后续接入批次，不进入首版 MVP 强依赖。

## i18n

所有页面文案必须进入 locale：

- 菜单名称
- tab 名称
- 表单 label
- placeholder
- 空状态
- toast
- validation message
- confirm message

技术字段展示规则：

- 主标签展示 `name_i18n` / `label_i18n`。
- `uuid` 不作为主展示。
- `namespace`、`code` 可作为二级 muted metadata 展示。

## 权限

页面权限建议：

- 数据字典查看：`metadata.dictionary:read`
- 数据字典管理：`metadata.dictionary:manage`
- 分类体系查看：`metadata.taxonomy:read`
- 分类体系管理：`metadata.taxonomy:manage`
- 标签查看：`metadata.tag:read`
- 标签管理：`metadata.tag:manage`
- 资源类型查看：`metadata.resource_type:read`
- 资源类型管理：`metadata.resource_type:manage`

菜单可见性由 read 权限控制。

按钮可见性由 manage 权限控制。
