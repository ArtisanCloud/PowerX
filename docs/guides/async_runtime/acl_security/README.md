# ACL / Security（async_runtime）

> 状态：已实现（基础治理）  
> 平台入口：`docs/guides/async_runtime/README.md`

## 1. 范围

1. Topic 动作授权（publish / subscribe / replay）
2. 主体标识（principal_type / principal_id）
3. 403 排查路径

## 2. 已实现入口

1. `GET /admin/event-fabric/acl/topic-matrix`
2. `GET /admin/event-fabric/acl/principal-matrix`
3. `POST /admin/event-fabric/acl`

## 3. 典型排查

1. 先确认 Topic 已注册（`event_topics`）
2. 再确认主体是否具备目标动作
3. 最后确认请求上下文（JWT tenant / principal）一致

## 4. 参考文档

1. `docs/guides/async_runtime/event_fabric/integration_playbook.md`
2. `docs/guides/async_runtime/event_fabric/naming_convention.md`

