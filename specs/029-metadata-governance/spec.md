# Feature Specification: Metadata Governance

**Feature Branch**: `029-metadata-governance`  
**Created**: 2026-07-12  
**Status**: Draft  
**Input**: User description: "请根据 docs/plan/metadata-governance 生成对应的 spec 文档"

## Clarifications

### Session 2026-07-12

- Q: 029-metadata-governance 的 MVP 是否包含首个业务模块迁移？ → A: 只做元数据治理中心、底座 API、权限、seed、插件消费契约；不迁移现有业务模块。
- Q: metadata seed 的触发方式怎么定？ → A: 同时支持显式命令和租户初始化流程触发；禁止应用启动时自动 seed。
- Q: 资源类型没有对象校验器时，是否允许写标签绑定？ → A: 不允许写入；资源类型缺少对象校验器时只能读定义，不能绑定标签。
- Q: 当前语言缺少翻译时，后台管理页面怎么显示？ → A: 显示 zh-CN 名称，同时明确标记当前语言缺失；API 返回缺失标记。
- Q: 引用登记失败时，业务写入怎么处理？ → A: 对已接入 metadata governance 的模块，业务写入必须失败并回滚。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 统一管理数据字典 (Priority: P1)

作为租户管理员，我希望在统一入口管理客户来源、客户等级、任务优先级等业务枚举，以便底座、插件、Agent 和业务页面使用同一份权威定义，而不是各模块各自维护。

**Why this priority**: 数据字典是最基础、最稳定的元数据治理对象。没有统一字典，后续分类、标签和插件接入都会继续产生重复定义。

**Independent Test**: 管理员创建一个字典命名空间和多个字典项，业务选择器能只展示启用项；停用项不再可选但历史已选值仍可读。

**Acceptance Scenarios**:

1. **Given** 租户管理员进入元数据治理页面，**When** 创建一个字典命名空间并添加多个字典项，**Then** 字典项按租户隔离、按排序展示，并可按名称或 code 搜索。
2. **Given** 一个字典项已被业务数据引用，**When** 管理员尝试删除该字典项，**Then** 系统拒绝硬删除并提示存在引用，只允许停用或先迁移引用。
3. **Given** 一个字典项被停用，**When** 用户打开新建或编辑业务表单，**Then** 该字典项不再作为新值可选；已保存历史值仍显示原名称。
4. **Given** 管理员编辑字典名称或描述，**When** 切换后台语言，**Then** 页面展示对应语言的名称，不把 code 或 UUID 作为主展示。

---

### User Story 2 - 维护可控分类体系 (Priority: P1)

作为租户管理员，我希望维护知识库目录、内容类目、商品分类等层级分类，以便业务对象能使用受控层级而不是自由文本路径。

**Why this priority**: 分类体系决定了内容组织和业务筛选的结构边界，需要和字典一样成为首版治理能力。

**Independent Test**: 管理员创建一个分类体系、添加多级节点、移动节点并验证层级深度、环检测和引用删除保护。

**Acceptance Scenarios**:

1. **Given** 租户管理员创建分类体系并设置最大层级，**When** 新建根节点和子节点，**Then** 系统按层级展示节点并阻止超过最大层级的创建。
2. **Given** 一个分类节点已有后代节点，**When** 管理员尝试把该节点移动到自己的后代下，**Then** 系统拒绝该移动并提示会形成循环。
3. **Given** 一个分类节点已被业务对象引用，**When** 管理员尝试硬删除该节点，**Then** 系统拒绝删除并提示引用摘要。
4. **Given** 父节点被停用，**When** 管理员保存变更，**Then** 系统提示子节点可选状态会受影响，并在业务选择器中体现停用结果。

---

### User Story 3 - 治理标签和标签绑定 (Priority: P1)

作为租户管理员，我希望统一管理客户标签、文件标签、Agent 标签等业务标注，并查看标签使用情况，以便标签可筛选、可统计、可审计、可合并。

**Why this priority**: 标签是现有系统最分散的元数据形态之一，直接影响客户、知识库、媒体和 Agent 上下文组织。

**Independent Test**: 管理员注册资源类型、创建标签、给资源绑定标签、查看使用次数、合并标签，并验证审计记录。

**Acceptance Scenarios**:

1. **Given** 某业务对象类型已注册为可打标签资源，**When** 管理员创建该资源类型下的标签，**Then** 标签按租户、命名空间和资源类型唯一。
2. **Given** 用户给业务对象绑定多个标签，**When** 保存绑定，**Then** 系统只接受已启用标签，并记录绑定人和绑定时间。
3. **Given** 两个标签表达同一含义，**When** 管理员执行标签合并，**Then** 源标签的绑定转移到目标标签，合并动作写入审计。
4. **Given** 一个标签已有绑定，**When** 管理员尝试硬删除该标签，**Then** 系统拒绝删除并提示使用次数。

---

### User Story 4 - 管理资源类型和引用完整性 (Priority: P2)

作为平台或租户管理员，我希望所有可被标签绑定或元数据引用的业务对象先注册资源类型，以便系统能校验资源存在性、租户边界和删除保护。

**Why this priority**: 标签绑定是多态关系，必须通过资源类型治理来避免无效 resource reference、跨租户绑定和不可删除数据。

**Independent Test**: 注册一个资源类型后才能写入标签绑定；未配置对象校验能力的资源类型只能查看，不能写入绑定。

**Acceptance Scenarios**:

1. **Given** 某业务对象类型尚未注册，**When** 用户尝试给该对象写入标签绑定，**Then** 系统拒绝并提示资源类型不存在。
2. **Given** 某资源类型已注册但不可写入绑定，**When** 用户尝试替换标签绑定，**Then** 系统拒绝并提示缺少对象校验能力。
3. **Given** 某资源类型已启用且对象属于当前租户，**When** 用户替换标签绑定，**Then** 系统完成绑定并更新使用统计。
4. **Given** 资源类型已有绑定数据，**When** 管理员尝试删除资源类型，**Then** 系统拒绝删除并要求先清理绑定。

---

### User Story 5 - 插件消费底座元数据 (Priority: P2)

作为插件开发者，我希望插件通过统一客户端读取底座字典、分类、标签并替换标签绑定，以便 delegated 模式使用底座权威元数据，local 模式使用同源 seed 调试。

**Why this priority**: 插件如果继续各自维护默认值，metadata governance 无法成为生态统一能力。

**Independent Test**: 一个插件在 delegated 模式读取字典项和标签并替换绑定；在 local 模式缺少 seed 时启动失败。

**Acceptance Scenarios**:

1. **Given** 插件运行在 delegated 模式，**When** 插件请求读取某个字典命名空间，**Then** 返回当前租户启用项，不读取插件私有 fallback。
2. **Given** 插件运行在 delegated 模式但缺少能力授权，**When** 插件请求读取或写入 metadata，**Then** 系统返回明确授权错误。
3. **Given** 插件运行在 local 模式且 canonical seed 缺失，**When** 插件初始化 metadata client，**Then** 初始化失败，不返回空列表或默认假数据。
4. **Given** 插件需要创建或管理字典、分类、标签，**When** 插件后台尝试用服务身份绕过管理权限，**Then** 系统拒绝该管理写入。

---

### User Story 6 - 元数据治理管理页面可用 (Priority: P2)

作为后台管理员，我希望在一个页面中按数据字典、分类体系、标签、资源类型分区管理元数据，并清楚区分无数据、无权限、未选择和加载错误。

**Why this priority**: 治理能力必须有可操作页面，否则只能依赖开发人员或脚本维护，无法成为日常运营工具。

**Independent Test**: 管理员进入元数据治理页面，分别操作四个 tab，验证筛选、空态、错误态、权限按钮和多语言展示。

**Acceptance Scenarios**:

1. **Given** 管理员拥有查看权限，**When** 进入元数据治理页面，**Then** 系统显示数据字典、分类体系、标签、资源类型四个分区。
2. **Given** 当前未选择命名空间或资源类型，**When** 页面需要展示右侧列表，**Then** 显示“未选择”状态而不是空列表。
3. **Given** 用户只有查看权限没有管理权限，**When** 页面渲染操作区，**Then** 管理按钮不可见或不可用。
4. **Given** 后端返回契约错误或授权错误，**When** 页面加载失败，**Then** 页面显示明确错误状态，不渲染成“暂无数据”。

### Edge Cases

- 字典命名空间、分类体系、标签 namespace 或资源类型重名时，系统必须按租户边界拒绝重复创建。
- 多语言字段缺少必填语言时，创建或更新必须失败。
- 请求语言没有对应翻译时，系统不得把 code 当作主名称静默展示；后台管理页面可显示 zh-CN 名称但必须明确标记当前语言缺失。
- 用户尝试使用 UUID、用户可见文案或自由文本作为 namespace/code/resource type 时，系统必须拒绝。
- 分类节点移动过程中发生并发修改时，系统必须拒绝保存并提示刷新最新树。
- 标签合并时源标签和目标标签相同，或跨资源类型合并，系统必须拒绝。
- 业务对象删除、归档或迁移时，引用登记必须同步变化；同步失败时业务写入不得部分成功。
- 已接入 metadata governance 的业务模块写入元数据引用时，引用登记失败必须导致业务写入失败并回滚。
- root/support 跨租户查看必须走独立入口，不得复用普通租户管理入口。
- 对外 web、mini-app、customer 场景不得复用后台 metadata 管理路径。
- 历史业务表中已有私有 tags 或字典值时，首版不自动回填；接入模块必须提交迁移说明后再迁移。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide a single metadata governance area for dictionary namespaces/items, taxonomies/nodes, tags/bindings, and resource types.
- **FR-002**: System MUST keep dictionary, taxonomy, tag, and resource type definitions tenant-scoped and prevent cross-tenant reads or writes through tenant administration flows.
- **FR-003**: System MUST require stable machine identifiers for namespace, code, and resource type, and MUST reject UUIDs or user-visible labels used as those identifiers.
- **FR-004**: System MUST store and expose user-visible metadata names and descriptions as multilingual values with a required primary language for the first release.
- **FR-005**: System MUST expose locale-specific display names while preserving the original multilingual values for administrators.
- **FR-006**: System MUST prevent silent fallback that displays code, UUID, or technical identifiers as the primary human-facing label when a required translation is missing.
- **FR-006a**: System MAY expose the required primary-language display name in administrator views when the requested locale is missing, but MUST return an explicit missing-locale marker so the UI can show a visible translation-missing state.
- **FR-007**: System MUST support dictionary namespace and item creation, editing, listing, enablement, disablement, archiving, and deletion only when no protected references exist.
- **FR-008**: System MUST treat dictionary namespace and item codes as immutable after creation.
- **FR-009**: System MUST support taxonomy creation, node creation, node editing, enablement, disablement, archiving, deletion protection, and node movement.
- **FR-010**: System MUST enforce taxonomy maximum depth and prevent circular hierarchy moves.
- **FR-011**: System MUST preserve historical readability for disabled dictionary items, taxonomy nodes, and tags while preventing their use in new selections.
- **FR-012**: System MUST require resource type registration before tags can be bound to a business object type.
- **FR-013**: System MUST reject tag binding writes when the resource type is disabled, missing, or cannot verify the target resource belongs to the current tenant.
- **FR-013a**: System MUST reject tag binding writes when the resource type does not have an enabled object validator; read-only resource type definitions remain listable.
- **FR-014**: System MUST support tag creation, editing, enablement, disablement, archiving, deletion protection, and merge operations.
- **FR-015**: System MUST record auditable events for metadata creation, update, disablement, archiving, deletion, merge, and binding changes.
- **FR-016**: System MUST maintain protected reference records for dictionary items, taxonomy nodes, and tag bindings so deletion conflicts can be detected without scanning every business table.
- **FR-016a**: System MUST make metadata reference registration part of the write consistency boundary for business modules that adopt metadata governance; registration failure MUST fail and roll back the business write.
- **FR-017**: System MUST reject hard deletion of referenced metadata and return a stable conflict response with a reference summary.
- **FR-018**: System MUST distinguish empty data, missing selection, insufficient permission, loading, and backend error states in the management experience.
- **FR-019**: System MUST control page visibility and management actions with separate read and manage permissions for dictionaries, taxonomies, tags, and resource types.
- **FR-020**: System MUST define capability-to-permission mappings for dictionary, taxonomy, tag, and resource type read/manage operations.
- **FR-021**: System MUST allow plugins in delegated mode to read dictionary items, read taxonomy nodes, read tags, resolve resource types, and replace tag bindings only through the governed metadata contract.
- **FR-022**: System MUST prevent plugin service identities from creating or managing dictionary namespaces, dictionary items, taxonomy nodes, tags, or resource types unless they use an authorized administrator path.
- **FR-023**: System MUST provide metadata seed through an explicit command for development/repair flows and a tenant initialization hook for new tenant bootstrap; application runtime startup MUST NOT create or seed metadata.
- **FR-024**: System MUST fail tenant initialization or local plugin metadata initialization when required seed definitions are missing or invalid.
- **FR-025**: System MUST expose enough filtering for administrators to find metadata by namespace, module, resource type, status, and text search.
- **FR-026**: System MUST require every business module that adopts metadata governance to provide a mapping plan for fields, namespaces, resource types, historical value handling, and reference registration.
- **FR-027**: System MUST keep external consumer interfaces for web, mini-app, and customer scenarios out of the first release scope.
- **FR-028**: System MUST produce a clear error when metadata objects are missing instead of accepting deprecated names, inferring namespaces from free text, or returning empty fallback lists.
- **FR-029**: System MUST preserve PATCH/update field presence across REST and gRPC contracts; gRPC update requests MUST use explicit presence semantics such as `optional` scalar fields or field masks for booleans, numeric values, enums, and strings that can be intentionally set to false, zero, disabled, archived, or empty.

### Key Entities

- **Dictionary Namespace**: Tenant-scoped grouping for a business enumeration, identified by stable namespace and module, with multilingual name and status.
- **Dictionary Item**: Tenant-scoped option inside a dictionary namespace, identified by stable code, multilingual label, sort order, status, and reference state.
- **Taxonomy**: Tenant-scoped hierarchical classification system, identified by stable namespace, module, multilingual name, maximum depth, and status.
- **Taxonomy Node**: Node within a taxonomy tree, identified by stable code and object UUID, with parent relationship, depth, path, multilingual label, sort order, status, and reference state.
- **Tag**: Tenant-scoped label for a resource type and namespace, identified by stable code, multilingual label, optional color, source, status, and usage state.
- **Tag Binding**: Relationship between a tag and a business object resource, scoped by tenant, resource type, resource UUID, and creator.
- **Resource Type**: Registered business object type that may participate in metadata binding, with module, multilingual name, status, and binding eligibility.
- **Metadata Reference**: Protected reference record connecting a metadata object to a business object field, used for deletion protection and reference summaries.
- **Metadata Seed Definition**: Canonical definition used to instantiate baseline dictionaries, taxonomies, tags, and resource types for a tenant or local plugin development.
- **Capability Permission Mapping**: Governance mapping that connects a metadata capability to the permission required for user, plugin, or Agent use.

### Assumptions

- The first release focuses on tenant administration and plugin consumption; public web, mini-app, and customer-facing metadata APIs are out of scope.
- Existing business modules will not be migrated in this feature; each module will adopt metadata governance through a later mapping and migration plan.
- Multilingual values require `zh-CN` in the first release, while other supported locales may be added by administrators.
- Plugins can consume metadata and replace tag bindings in delegated mode, but plugin service identity cannot manage metadata definitions.
- Seed execution is explicit and observable through command or tenant bootstrap flows; application startup does not perform hidden metadata seeding.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An administrator can create a dictionary namespace with five items, disable one item, and verify the disabled item is unavailable for new selection within 5 minutes.
- **SC-002**: An administrator can create a taxonomy with at least three levels and successfully move a node while the system blocks invalid circular moves in 100% of tested cases.
- **SC-003**: An administrator can create tags, bind them to a registered resource, and complete a tag merge with an audit record visible for the merge action.
- **SC-004**: 100% of attempted hard deletions for referenced dictionary items, taxonomy nodes, or tags are rejected with a conflict response and reference summary.
- **SC-005**: Users without manage permission can view permitted metadata but cannot see or execute management actions in all metadata governance tabs.
- **SC-006**: Locale switching displays translated metadata names for all records that provide that locale, and missing required primary-language labels block create/update operations.
- **SC-007**: A delegated plugin can read dictionary items, taxonomy nodes, and tags for its tenant and receives a clear authorization error when its permission is missing.
- **SC-008**: Local plugin metadata initialization fails clearly when canonical seed definitions are absent or invalid.
- **SC-009**: Administrators can distinguish “not selected,” “no data,” “no permission,” “loading,” and “error” states in each governance tab during acceptance testing.
- **SC-010**: No metadata governance acceptance test requires users to copy or use UUIDs as the primary visible label.
