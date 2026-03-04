# FAQ（async_runtime）

> 状态：已建立（持续补充）  
> 平台入口：`docs/guides/async_runtime/README.md`

## 1. 为什么页面看起来“刷新了一下”？

优先检查是否是 WS 重连导致的状态同步请求，而不是前端轮询。  
参考：`docs/guides/async_runtime/event_fabric/integration_playbook.md`

## 2. 为什么 Queue 计数是 0，但任务执行成功？

`stats` 是运行态瞬时值，可能很快归零。  
请以 `messages.history` 为追溯依据。

## 3. 为什么会出现 `tenant_key=global`？

`global` 是系统级队列分片值，不是 topic 字段。  
用于系统公共任务（例如通知分发）。

## 4. 遇到 403 unauthorized 怎么查？

先查 Topic 是否存在，再查 ACL 动作是否授权，最后查主体与 JWT 上下文。

