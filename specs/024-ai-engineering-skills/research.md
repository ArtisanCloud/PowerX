# Phase 0 Research — PowerX Skills 管理与治理

## Decision 1: 第三方导入入口首版限定为“上传 Bundle + 来源登记”
- **Decision**: 首版第三方 Skill 仅支持上传 Bundle 导入，并记录 `source_url/source_ref` 作为审计元数据，不执行在线拉取。
- **Rationale**: 与澄清结论一致，避免引入远程仓库可用性与供应链风险，且可快速落地管理闭环。
- **Alternatives considered**:
  - 直接 Git 拉取：实现复杂且安全边界扩大。
  - 仅上传不记录来源：审计追溯能力不足。

## Decision 2: 调用未指定版本时默认路由到“最新已发布版本”
- **Decision**: tenant/agent 调用省略 version 时，统一解析为该 `skill_id` 的最新 `published` 版本。
- **Rationale**: 降低调用接入复杂度，并与“回滚切换发布指针”机制天然兼容。
- **Alternatives considered**:
  - 强制显式 version：接入方负担较重。
  - 记忆租户上次版本：语义不稳定，排障复杂。

## Decision 3: 首版全量人工审批发布
- **Decision**: 所有 Skill（builtin/plugin/third_party）都需人工审批后才能从 draft 进入 published。
- **Rationale**: 统一治理边界，减少策略分叉，降低初期运营风险。
- **Alternatives considered**:
  - 按来源差异化审批：规则复杂，易造成误配。
  - 全自动发布：不满足首版风险控制目标。

## Decision 4: 完整性校验门槛为 checksum 强制、signature 可配置强制
- **Decision**: 发布前必须通过 checksum 校验；signature 默认可选，但必须支持策略开关升级为强制。
- **Rationale**: 兼顾首版落地速度与企业级安全可升级性。
- **Alternatives considered**:
  - checksum+signature 都强制：生态导入门槛过高。
  - 两者都可选：无法满足可信发布要求。

## Decision 5: 官方固有 Skills 来源为后端内置目录表
- **Decision**: 官方 catalog 由后端内置维护并随平台版本发布。
- **Rationale**: 版本一致性和可控性最佳，避免运行时外部依赖导致目录漂移。
- **Alternatives considered**:
  - 定时同步官方仓库：链路复杂且需要额外故障治理。
  - 人工导入并打标签：长期维护成本高。
