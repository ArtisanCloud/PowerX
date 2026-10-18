# IAM 连续授权基线实施入口

日期：2026-10-15。状态：`core_source_implemented`；Framework 和 AI Craft v2 待适配，安装态验收未完成。

权威正文见 [IAM 连续授权基线跨仓实施指导](../guides/features/iam-authorization-baseline/guide.md)。本文只提供合同检索入口，不复制方案。

目标能力为 `com.corex.iam.authorization_baseline.service_read`，固定 `INVOKE core://iam/authorization-baseline`，operation 为 `read_member_baselines`。已交付 [正式 schema](../../specs/026-iam/contracts/authorization-baseline.openapi.yaml)、[platform capability](../../backend/config/platform_capabilities/iam-authorization-baseline.yaml) 和 `backend/internal/service/authorization_baseline` typed invoker。本地已完成 capability-check/seed；实际开发 Host key 仍为 not_granted。业务库迁移、受控初始化和服务重启尚未执行；本文不代表已完成安装态联调。

三方须共同对齐连续区间与决策分段、授权源并发锁、一致性批量读取、`granted_at` 与 `visible_since`、首期记录时间语义及范围/Root 边界。插件目标合同升级为 `order_content_authorization.v2`，不能用初始化 T0 填充旧 GrantedAt。

实施指导 1.2 补入完整实时范围：WS 实际下行发送前重验证、排队撤权与重连不确认已读；Registry／Bundle 和 Skeleton 启用门禁；scaffold／CLI 内嵌模板及新生成插件验证；delegated worker 从宿主分页目录读取成员、批量基线筛选，禁止本地 IAM 旁路。对应 C06、F02–F04、P02–P03、A02 与 B19–B24；这些新增交付／验收仍待各仓完成，不回退为旧版 design_only，也不宣称完整实时接入已交付。

2026-10-18 补充：Selector 自动添加的 `tool_scope` 已与调用方原始 context 分离，修复正常 typed 基线查询被误报为 `IAM_BASELINE_INVALID_ARGUMENT` 的问题。完整调用链回归及边界见 [Capability 调用上下文](capability-invocation-context.md)。Framework 对外合同保持不变；本次源码修复仍需运行实例重新编译／启动后生效。
