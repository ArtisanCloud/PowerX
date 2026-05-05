# WebSocket 实现指南（ws-bus）

## 概述

本文描述 PowerX 当前 ws-bus WebSocket 的实现与联调方式，适用于插件进度/通知等事件场景。

## 1. 当前接入路径

1. 浏览器连接：`wss://<domain>/api/ws?tenant_uuid=<tenant_uuid>&authorization=Bearer+<token>`
2. 反代：Nginx 将 `/api/ws` 升级转发到后端服务。
3. 协议：连接建立后发送 `subscribe`（`topics[]`）。

## 2. Nginx 关键配置

```nginx
location /api/ws {
    proxy_pass http://127.0.0.1:8080/api/ws;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

## 3. 订阅协议

```json
{ "type": "subscribe", "topics": ["ai_craft.shopify.sync.progress.<tenant_uuid>"] }
```

失败返回：
```json
{
  "type": "error",
  "payload": {
    "code": "permission_denied",
    "message": "subscription rejected",
    "detail": "topic not allowed"
  }
}
```

## 4. 与 grant 的关系

1. grant 是“授权准备”，不是自动订阅。
2. 只有 `grant.data.topics` 命中目标 topic，订阅才稳定通过。
3. 若 grant 仅 fallback（`topics: []`），订阅可能被拒绝。

## 5. 验收标准

1. grant 200 且 `data.topics` 非空。
2. ws `subscribe` 无 `topic not allowed`。
3. 宿主日志出现 `stage=subscribed` 与 `stage=emit`，并有 `emitted_count > 0`。

## 6. 常见故障定位

1. `401/400 on grant`
- 查插件 runtime grant 日志与宿主鉴权日志。

2. `grant success but no event`
- 先看 grant 是否仅 fallback；
- 再看 ws subscribe 是否被拒绝。

3. `topic not allowed`
- 核查 topic 是否已在 registry 精确注册；
- 核查当前连接租户与 topic tenant 是否一致。
