# Capability 调用方参数与内部路由元数据

2026-10-18。适用于 Selector → InvocationService → Core typed invoker／插件通用调用 envelope，不修改 Framework 对外请求合同。

## 参数归属

- `CapabilityInvokeRequest.Context` 是调用方原始输入。Selector 复制该 map，不能通过添加、覆盖或删除键，把它改造成内部生成的可信上下文。
- Selector 解析的 scope 经现有策略选择与授权检查后，保存在内部 `InvocationInput.ResolvedToolScope`。该字段禁止 JSON 序列化／反序列化，不作为新的客户端参数，也不代表额外授权。
- `CoreCapabilityInvokeInput.Context` 只接收调用方原始 context。要求空 context 的固定 typed binding 继续拒绝任何调用方扩展；不能仅按键名移除 `tool_scope`，也不能直接取消空 context 检查。
- 插件通用 REST 调用 envelope 在组装 `metadata` 时使用内部解析的 scope，保留原有 `metadata.tool_scope` 行为和其他调用方元数据。调用方同名键不能覆盖 Selector 已解析的值。
- 服务凭据、租户身份、发布状态、租户登记和实际 grant 继续由原鉴权链判断，不从这个内部字段推导身份或绕过授权。

## 本次修复

此前 Selector 总是将 `tool_scope` 写入 context，即使调用方未传 context。IAM 基线 invoker 的严格校验因此把正常请求拒绝为 `400 / IAM_BASELINE_INVALID_ARGUMENT`。通知创建和 Media typed binding 也有同类空 context 约束，修复放在公共调用链中。

回归测试使用隔离数据库及真实 PolicyGenerator → Selector → AuthorizationService → InvocationService → Router → CoreCapabilityMux → 基线服务，覆盖默认 scope、显式 scope、intent 映射、三项业务权限、未初始化状态、调用方伪造 context／代理参数及撤销服务 grant。插件 REST envelope 另有兼容回归。测试不替代已安装服务重启后的真实请求验收。

本次不改 HTTP 路由、typed operation DTO、能力 YAML 或数据库 schema，无需为此重打 AI Craft 包或升级 Framework SDK。运行中的 Core 需由服务所有者重新编译／启动后生效。

## 当前实例交接

目标租户 `6b5d0240-9920-46da-b707-88200e0f51ea` 的三项缺失业务权限基线已使用已安装插件的真实 pending guard 初始化；只读复查均为 ready。Core 未修改插件启用状态或宿主铃铛开关，用户服务未重启。原始预检／apply／复查 JSON、guard 及验证边界见 [交接收据](evidence/iam-baseline-selector-context-20261018.json)。
