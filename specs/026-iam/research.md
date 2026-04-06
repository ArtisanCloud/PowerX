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
