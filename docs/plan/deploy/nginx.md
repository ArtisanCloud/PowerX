# Nginx 同机同域反代方案

## 1. 路由目标

- `/` -> `http://127.0.0.1:3000`（web-admin）
- `/api/` -> `http://127.0.0.1:8077/api/`（backend）
- `/_p/` -> `http://127.0.0.1:8077/_p/`（插件前端/接口）

## 2. 参考配置

```nginx
server {
  listen 80;
  server_name admin.powerx.example.com;
  return 301 https://$host$request_uri;
}

server {
  listen 443 ssl http2;
  server_name admin.powerx.example.com;

  ssl_certificate /etc/letsencrypt/live/admin.powerx.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/admin.powerx.example.com/privkey.pem;

  proxy_set_header Host $host;
  proxy_set_header X-Real-IP $remote_addr;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  proxy_set_header X-Forwarded-Proto $scheme;

  location /api/ {
    proxy_pass http://127.0.0.1:8077/api/;
    proxy_http_version 1.1;
  }

  location /_p/ {
    proxy_pass http://127.0.0.1:8077/_p/;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 3600s;
  }

  location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 3600s;
  }
}
```

## 3. 验证项

- `curl -I https://admin.powerx.example.com/` 返回 200
- `curl -I https://admin.powerx.example.com/api/v1/health` 返回 200
- 插件页面 `/_p/<plugin_id>/admin/` 可打开
- 登录态下插件 iframe 内 API 请求携带 Authorization

## 4. 常见问题

- 如果插件 iframe 跳转登录页，优先检查宿主与插件会话桥接（postMessage）
- 如果 `/_p/*` 404，检查 backend 插件路由是否启用且插件已 enable
- 如果 WebSocket/SSE 中断，提升 `proxy_read_timeout` 并检查 Upgrade 头

