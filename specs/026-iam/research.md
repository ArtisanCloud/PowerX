# Phase 0 Research: IAM 用户与角色 RBAC 统一能力

## Decision 1: 角色判定单一事实来源以服务端 me/context 为准

- Decision: 页面关键分流（root/admin/member）统一以 `/api/v1/admin/user/auth/me/context` 返回为准，本地缓存仅作短期加速。
- Rationale: 避免前端缓存与真实授权状态漂移导致的误判与越权风险。
- Alternatives considered:
  - 前端本地角色常驻缓存为准：切租户/跨标签后易失真。
  - 页面独立调用多个接口拼装：一致性与延迟成本更高。

## Decision 2: 用户管理页动作语义显式拆分

- Decision: 将“查看详情”“切换租户”“进入其他业务页”设计为独立动作与独立入口，不再复用同一点击行为。
- Rationale: 消除误触与认知歧义，降低“点击即跳转 dashboard”类问题。
- Alternatives considered:
  - 维持行点击复合动作：对操作员不透明，回归风险高。

## Decision 3: root 与租户管理员能力边界双轨表达

- Decision: root 保留跨租户管理能力；租户管理员严格限制在当前租户；成员默认只读/受限。
- Rationale: 与多租户最小权限原则兼容，同时保留平台运维必要能力。
- Alternatives considered:
  - root 也强制绑定单租户：会破坏平台级运维与排障场景。
  - 租户管理员可跨租户：违反隔离边界。

## Decision 4: 先修复可观测与文档，再扩展复杂授权模型

- Decision: 本迭代优先补齐可回归的行为规范、接口契约和排障文档，不引入额外复杂策略引擎。
- Rationale: 当前痛点是语义不一致与行为漂移，先收敛主路径更稳妥。
- Alternatives considered:
  - 直接重构完整 RBAC 引擎：投入大、验证链路长、短期收益低。

## Decision 5: gRPC 合同与 HTTP 合同同步规划

- Decision: 即使本轮主要问题在 HTTP + 前端，也同步给出 gRPC 管理合同草案，保持 CoreX dual-transport 约束一致。
- Rationale: 避免后续补齐 gRPC 时再做二次语义对齐。
- Alternatives considered:
  - 仅写 HTTP 合同：与宪章双传输要求不一致。

## Decision 6: SaaS 注册创建租户而不是在当前租户下加成员

- Decision: 新增独立公开入口 `/api/v1/public/saas/signup`，语义是创建新 tenant 并把注册用户初始化为该租户 owner/admin/member。
- Rationale: 现有 register 更接近“已有租户上下文下注册成员”，不能承载 SaaS 自助开通租户的事务边界。
- Alternatives considered:
  - 复用现有 register：语义混乱，容易把“创建账号”和“创建租户”耦合到当前上下文。
  - root 后台手动开通：不满足 SaaS 自助增长模型。

## Decision 7: root 保留 system tenant member 但不等于业务租户 admin

- Decision: 保留 root user、`system` tenant member/admin 和 setup 完成记录；`system` member 是平台身份锚点，用于登录 token、审计、STS、API Key Profile、setup 初始化和历史安装兼容。代码语义上 root 默认进入 Platform Console，不自动拥有业务租户 owner/admin 能力。
- Rationale: 不破坏已有初始化数据，同时切断 root 与租户业务后台的隐式权限混用。
- Alternatives considered:
  - 删除 root 的 system member：会破坏登录上下文和历史安装兼容。
  - 继续让 root 视为所有租户 admin：违反 SaaS 最小权限边界。

## Decision 8: 插件采用全局包 + 租户实例模式

- Decision: 插件物理包继续全局安装到 `plugins/installed/<plugin_id>/<version>`，租户启用状态由 `TenantPluginInstance` 表达，菜单和代理入口按当前租户实例过滤。
- Rationale: 避免某个租户安装/停用插件影响其他租户，同时保留全局版本治理能力。
- Alternatives considered:
  - 每个租户复制一份插件物理包：存储和升级成本高，版本治理复杂。
  - 只看全局插件 enabled：SaaS 隔离不足，未启用租户可以误访问插件入口。

## Decision 9: 插件运行时进程按全局插件包维度管理

- Decision: PowerX 节点内存里的插件运行进程按 `plugin_id` 管理，后端进程 key 为 `plugin_id`，admin 进程 key 为 `plugin_id_admin`；多个租户启用同一插件时共享同一组进程。
- Rationale: 当前 supervisor 和 dynamic router 已按全局 plugin id 管理进程与路由；SaaS 隔离应该放在请求上下文、事件 payload、租户实例配置和数据访问层，而不是为每个租户复制进程。
- Alternatives considered:
  - 每租户启动一组插件进程：资源成本高、端口/健康检查/升级复杂，且物理包版本治理更难。
  - 用进程启动 env 固定某个租户：会导致共享进程误把所有请求当成同一租户，不符合 SaaS 隔离。

## Decision 10: 历史数据先巡检再迁移

- Decision: SaaS 语义上线前新增只读 IAM 巡检报告；可推导的 owner 缺失自动补齐，无法推导的 admin 缺失只报告不修复。
- Rationale: 服务器已有组织、部门、角色数据不能手动破坏；自动迁移必须可解释、可审计。
- Alternatives considered:
  - 手动改生产数据库：不可审计，风险高。
  - 静默 fallback 到 root 代管缺失租户：会掩盖数据问题，继续扩大 root 权限边界。
