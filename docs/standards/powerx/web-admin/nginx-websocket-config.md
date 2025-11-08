# Nginx WebSocket 代理配置

在生产环境中，需要在 Nginx 中配置 WebSocket 代理来处理 `/api/agents/stream/ws` 路径。

## 配置示例

```nginx
# 在你的 server 块中添加以下配置
location /api/agents/stream/ws {
    proxy_pass http://127.0.0.1:8077;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header Origin $http_origin;
    proxy_set_header Sec-WebSocket-Protocol $http_sec_websocket_protocol;
    proxy_read_timeout 3600;
    proxy_send_timeout 3600;
}

# 其他 API 路径的常规代理
location /api/ {
    proxy_pass http://127.0.0.1:8077;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

## 关键配置说明

- `proxy_http_version 1.1`: WebSocket 需要 HTTP/1.1
- `Upgrade` 和 `Connection`: WebSocket 握手必需的头部
- `Sec-WebSocket-Protocol`: 透传子协议头部，用于传递认证信息
- `proxy_read_timeout` 和 `proxy_send_timeout`: 设置较长的超时时间，避免长连接被断开

## 测试配置

配置完成后，可以使用以下命令测试 WebSocket 连接：

```bash
# 测试 WebSocket 连接
websocat "wss://yourdomain.com/api/agents/stream/ws?probe=1" \
  --header "Sec-WebSocket-Protocol: bearer.your_base64url_token"
```
