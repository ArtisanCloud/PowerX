# Customer / External Identity 真实运行验收

日期：2026-09-27。Core 由用户重启，本轮 HTTP 监听进程 PID 87136。

## 执行方式与结果

使用运行中的 `http://127.0.0.1:8077` 和真实 STS gRPC `127.0.0.1:9001`。创建临时插件实例，通过真实 STS Exchange 获取 token；API Key 使用临时精确 grant，并绑定相同插件命名空间。没有伪造 JWT。

```sh
cd backend
POWERX_CUSTOMER_TEST_CONFIG="$PWD/etc/config.yaml" \
POWERX_LIVE_TEST_URL=http://127.0.0.1:8077 \
POWERX_LIVE_TEST_STS=127.0.0.1:9001 \
go test ./tests/contract/plugin_grants -run '^TestLiveCustomerProfileUpdateAPIKeyAndSTS$' -count=1 -v
```

结果：PASS。临时客户、Contact、身份、membership、API Key/profile/grant 及临时插件实例配置已按测试 cleanup 清理；调用审计与 trace 保留。没有更改既有插件的 STS grant。

验证事实：

- API Key、STS 均能更新客户并重新读取；未传邮箱保留、显式空电话清空；type 和 primary_contact_uuid 不变。
- 非法 UUID/状态、禁止修改 type/主联系人、调用方传 tenant 均返回 400。
- 两种凭证均完成 Lookup（未绑定/已绑定）、ListByCustomer、Bind、CreateAndBind（新建/已关联重放）。
- 重复 Bind/CreateAndBind 返回相同 UUID，不覆盖人工资料。
- 身份绑定冲突 409；跨租户绑定/客户更新 404；撤销精确 API Key grant 或 STS 实例 grant 后原凭证返回 403。
- 原 Contact 和登录身份资料保持不变。

这是实际 Core 服务凭证与网关验收，不代表插件安装生命周期、插件管理 UI 或所有插件 Local adapter 已验收。并发 8 请求只创建一套聚合、故障完整回滚和只读不补写另由实际 PostgreSQL 集成测试覆盖。

## 真实 trace ID

| 操作 | HTTP | trace_id |
| --- | --- | --- |
| `grant-status` | 200 | `98c1b111-8c43-4a89-9c20-4037139ffad4` |
| `grant-status` | 200 | `61c6c90d-134f-4174-8d54-fb3db7ef1909` |
| `api_key_create` | 200 | `858e1364-ac77-4bb0-b067-2d2876cc6c8f` |
| `api_key_prepare_phone` | 200 | `f1236bda-4b98-4d40-b077-f3382dea415e` |
| `api_key_update` | 200 | `4a977cfb-ba6a-460f-a38d-ab2247a9bf95` |
| `api_key_reread` | 200 | `ace2ac86-a758-4ecb-bc57-8598fbbb5eb7` |
| `api_key_invalid` | 400 | `6894d6ea-3d29-40c5-874f-fc3ee0043ab4` |
| `api_key_invalid` | 400 | `ac84b147-6b87-4cfa-99d6-0bd452932756` |
| `api_key_invalid` | 400 | `a7952c02-8ff7-4476-be7a-a450e4b5f7d5` |
| `api_key_invalid` | 400 | `c3fec731-8459-410d-87db-fc094b51a018` |
| `api_key_invalid` | 400 | `e676dc97-25cc-43d0-a6dc-508f5711ea0d` |
| `sts_prepare_phone` | 200 | `e165d46b-9282-4e3e-8e43-0d0bfef44959` |
| `sts_update` | 200 | `9e542c17-258e-4830-abe4-ec3adf6c3884` |
| `sts_reread` | 200 | `51fc057c-50fe-42a8-818b-efa4753167cf` |
| `sts_invalid` | 400 | `54f68c93-c986-4c1c-9da8-8e615d07dcf0` |
| `sts_invalid` | 400 | `f156695b-e466-45e8-bfad-745a65c0e499` |
| `sts_invalid` | 400 | `afbe988e-3541-4883-9fd3-71ef5fffc699` |
| `sts_invalid` | 400 | `e82f9a2e-2863-4985-9393-e0b5972bd517` |
| `sts_invalid` | 400 | `cee4e5b4-c5df-45db-b600-9fda72df2792` |
| `api_key_identity_lookup_missing` | 200 | `df91f7d5-7c0b-46ad-a9d3-f32a22471912` |
| `api_key_identity_bind` | 200 | `9ca2be95-6904-4fce-b52a-3df93ea16536` |
| `api_key_identity_bind_repeat` | 200 | `fd1d56a5-779c-49d2-befa-2615228a435e` |
| `api_key_identity_lookup_found` | 200 | `782d0e63-0e9e-40d9-b69a-9b9e4399d2d0` |
| `api_key_identity_list` | 200 | `5be9d212-1331-4fe3-8431-26897ff858cc` |
| `api_key_identity_create_existing` | 200 | `688c022c-62ae-487d-bb9f-4ba7783250cf` |
| `api_key_identity_create_new` | 200 | `b7a0729c-4841-49c1-b930-3613d9384673` |
| `api_key_identity_create_new_repeat` | 200 | `706f9d40-aab5-4292-87f9-0079e5f37830` |
| `api_key_identity_conflict` | 409 | `96bd9d38-87c7-4d20-8052-25581cc842a0` |
| `sts_identity_lookup_missing` | 200 | `bd8e63b8-c941-49f1-942d-2ad6c86b64ab` |
| `sts_identity_bind` | 200 | `516b4ccd-2afa-46fc-a089-c98ef800b00a` |
| `sts_identity_bind_repeat` | 200 | `b04a44e7-370a-43dc-aaea-265d0a8222a7` |
| `sts_identity_lookup_found` | 200 | `2b95ebc6-f495-4bb8-9e96-92957e02f816` |
| `sts_identity_list` | 200 | `58827e90-c935-4785-98f9-77499f4f7a88` |
| `sts_identity_create_existing` | 200 | `91707c44-49ff-4c24-9d87-727b251b5a53` |
| `sts_identity_create_new` | 200 | `3030aa7e-251b-4115-bb05-c65af44673b2` |
| `sts_identity_create_new_repeat` | 200 | `42019a02-330e-4a6f-8d8a-db1e32530918` |
| `sts_identity_conflict` | 409 | `e93faa57-e269-4cc7-a21a-07ff02f8db05` |
| `identity_cross_tenant_bind` | 404 | `00902117-7e5e-497e-a66f-59fe2c6ae66f` |
| `cross_tenant` | 404 | `6f306057-6690-4325-9d81-8113f7ca7f5f` |
| `api_key_identity_no_grant` | 403 | `9522368f-a844-445a-9b7b-328f322955d0` |
| `grant-status` | 200 | `962bdf96-8633-4ce6-ae4f-80ea36d5b4ab` |
| `api_key_no_grant` | 403 | `c95244c6-19f8-48d6-9359-fea747612787` |
| `grant-status` | 200 | `c551d452-3757-40c1-b89a-8b1b17811c8d` |
| `sts_identity_no_grant` | 403 | `10a71579-d3e1-44e4-90a8-fec33c6c514e` |
| `sts_no_grant` | 403 | `e386c18e-9508-42c5-b0c1-171ba7946e9b` |

## 当前 skeleton 开发 Host Key

额外使用原有 skeleton `PX_GATEWAY_API_KEY` 调用全新随机 subject 的只读 Lookup，返回 HTTP 200、`found:false`；trace `38dbcfd3-afd9-4ddf-9946-388aaa96a9e4`。证明用户确认的 `com.powerx.plugins.base` 精确 grant 映射在真实进程生效。该请求没有创建业务数据。

合同：[客户外部身份管理](customer-external-identity-management.md)、[客户资料更新](customer-service-update.md)。
