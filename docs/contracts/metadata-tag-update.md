# Tenant Host 标签更新

## 故障与修复

2026-09-27，trace `c3e74bab-6ebf-47b4-985d-a046c5a53ebe` 的
`PATCH /api/v1/tenant/metadata/tags/6978bf18-e077-403e-a1c1-2cfb5dfba730`
在路由匹配阶段返回 404。原来仅 Admin 注册了 PATCH，Tenant Host 与正式能力声明均遗漏。

Core 补齐 Tenant PATCH，复用 `updateTag` / `Service.UpdateTag`，使用 `tag/manage` 服务授权。
本次无数据库结构变更，无需 migrate。代码与能力声明须加载到新进程。

## REST 合同

- `PATCH /api/v1/tenant/metadata/tags/{tag_uuid}`
- Capability：`com.corex.metadata.tag.manage`
- API Key：`_scope.metadata.tag.manage`，action=`manage`，resource=`api/tag`。
- STS：当前租户插件实例需授权上述 capability；普通 Admin RBAC 不能代替服务 grant。
- 同时检查能力已发布、租户已登记及当前凭证实际 grant。

```json
{
  "label_i18n": {"zh-CN": "重点客户", "en": "Key customer"},
  "description_i18n": {"zh-CN": "需要优先跟进", "en": "Priority follow-up"},
  "color": "#22c55e",
  "status": "enabled"
}
```

四个字段均可选；未传字段保持原值，`color: ""` 可清空颜色，空 description map 可清空描述。
传入的多语言 map 替换该字段原值，不做逐语言合并。标签名称须含非空 `zh-CN`；
status 为 `enabled` 或 `disabled`。租户来自可信认证上下文，禁止 body/query/header 覆盖。
不可借此修改 code、namespace 或 resource_type。成功响应为 `data.payload` 标签对象。
通过 `GET /api/v1/tenant/metadata/tags`（read grant）重新查询确认持久化结果。

错误：无凭证 401，无 grant 403 `METADATA_FORBIDDEN`，跨租户或不存在的标签 404
`METADATA_NOT_FOUND`，非法 UUID/状态/名称或租户覆盖 400。Core 返回 trace 响应头，
插件应保留 HTTP 状态、业务错误码和 trace ID，避免把所有失败显示为同一种提示。

## 类型化 Core binding

同一 capability 另提供固定 `core://metadata/tags` + `INVOKE`，用于 `/api/v1/tenant/invocations`：

```json
{
  "capability_id": "com.corex.metadata.tag.manage",
  "preferred_protocol": "core_internal",
  "payload": {
    "method": "INVOKE",
    "endpoint": "core://metadata/tags",
    "body": {
      "operation": "update",
      "tag_uuid": "<UUID>",
      "color": "#22c55e"
    }
  }
}
```

此 binding 当前只支持 update，业务 DTO 拒绝未知字段、租户/actor/plugin 覆盖与自由代理参数。
成功标签位于 `data.payload.item`。已有 REST Host 调用保持可用。

## 验收

- HTTP 回归 `TestTenantTagUpdateAPIKeyAndSTS`：真实注册路由、API Key/JWT 中间件、数据库授权和服务；
  测试内签名 STS JWT，不把它当成真实 Exchange 验收。
- 服务回归 `TestTagCapabilityUpdateAndStrictContract`：typed 更新、未知字段拒绝、错误目标、404 与撤销授权。
- 真实运行验收 `TestLiveMetadataTagUpdateAPIKeyAndSTS`：临时 API Key、真实 gRPC STS Exchange、
  PATCH 后读取、typed update、跨租户拒绝及撤销授权。仅使用生成的 fixture，结束清理，凭证不打印。

```bash
POWERX_METADATA_TEST_CONFIG="$PWD/backend/etc/config.yaml" \
POWERX_LIVE_TEST_URL=http://127.0.0.1:8077 \
POWERX_LIVE_TEST_STS=127.0.0.1:9001 \
go -C backend test ./tests/contract/plugin_grants \
  -run '^TestLiveMetadataTagUpdateAPIKeyAndSTS$' -count=1 -v
```

2026-09-27 已完成：

- metadata service、HTTP subject validator、API-key permission、metadata/plugin-grants contract、tenant invocation handler 六组测试通过。
- `make capability-check`：declared=1018，rest_routes=917，通过。
- `make capability-seed`：1018 capabilities 发布成功。
- Core 构建通过；本次变更 `git diff --check` 通过。
- skeleton 开发 Host Key 实查 read/manage 均 `granted`，HTTP 200，
  trace `a70222a9-894f-4eea-a767-95eb915b57f4`。

用户重启后已完成真实运行验收：8077 进程 PID `50607`，启动时间 2026-09-27 21:58:44。
`TestLiveMetadataTagUpdateAPIKeyAndSTS` 通过，使用真实 API Key 和 gRPC STS Exchange 获取的令牌。
PATCH 与 typed update 均成功；重新读取一致、颜色可清空、未传字段保留；非法输入 400，
跨租户 404，撤销 grant 后原凭证 403。临时标签、插件实例凭证和 API Key fixture 已清理。

| 请求 | HTTP | trace ID |
| --- | --- | --- |
| grant_status | 200 | `fe05036e-3d5a-4692-945a-4367a9946842` |
| grant_status | 200 | `179d81c5-8481-4983-8a5d-3a20835dbcff` |
| api_key_patch | 200 | `6f451cec-4471-4be1-8af7-fffea1316425` |
| api_key_reread | 200 | `df00c92b-06de-41d3-b4d2-8e179d9d332a` |
| api_key_clear | 200 | `d57ae59c-80d0-4076-98f9-7eecc2ddcaf5` |
| api_key_typed_update | 200 | `bd56cbc0-fda4-4444-a1dc-1de8921439a0` |
| api_key_invalid | 400 | `82f5f894-02f4-465d-ac5c-bf775a886052` |
| api_key_invalid | 400 | `aeabbe6a-8be7-4cb1-8322-7c8b3c9becd0` |
| api_key_invalid | 400 | `01124fbf-9d39-470c-9288-d869b263c944` |
| sts_patch | 200 | `7f7d1934-9975-475d-9681-95c0b62675a3` |
| sts_reread | 200 | `bede4881-7a61-4ab1-8acc-50f683c28fb7` |
| sts_clear | 200 | `7af486cd-a0e7-4988-aab0-875883c32f7a` |
| sts_typed_update | 200 | `c999c1b9-6618-41c5-a97e-699310b33d4f` |
| sts_invalid | 400 | `0ec5f928-1ed5-4ec1-8544-54b42e8feb86` |
| sts_invalid | 400 | `eea7aaea-8aea-4a21-9911-603ea2daa1ab` |
| sts_invalid | 400 | `11ce28b5-30cb-4cd9-b577-5065a84e3ab1` |
| sts_cross_tenant | 404 | `7a2659a1-42b3-42a1-8cf7-c935c75cedda` |
| api_key_cross_tenant | 404 | `923ebb04-d52b-4403-b11a-a187879c9ab0` |
| api_key_revoked | 403 | `fe08ee01-9d88-47a1-ab14-040cf31e38a9` |
| sts_revoked | 403 | `65dec4c7-dac6-41e4-a170-4973c74cb995` |

插件错误展示与编辑表单多语言布局由 PowerXPlugin 侧完成，此修复没有修改插件页面。
