# 部署选项与 Nginx 配置

> 总结 PowerX Web Admin 的部署形态：Vercel/静态托管、Node SSR、Docker + Nginx 反向代理，并附上 WebSocket/SSE 所需的配置要点。

---

## 1. 部署模式对比

| 模式 | 适用场景 | 优点 | 注意事项 |
| --- | --- | --- | --- |
| 静态托管 (`nuxt generate`) | Demo、文档站 | 部署简单、成本低 | 需要后端 API 代理；实时功能依旧依赖后端 |
| Vercel / NuxtHub | 快速上线、预览环境 | 内置 SSR、ISR、Edge 函数 | 需配置环境变量、付费计划支持 WebSocket |
| Node SSR（自托管） | 生产环境、可控基础设施 | 全面控制、易与后端同域部署 | 需运维 Node 进程、结合 Nginx/PM2 |
| Docker + Nginx | 标准化部署 | 易于横向扩展、配合 K8s | Nginx 需配置 WS/SSE 代理 |

---

## 2. Node SSR 部署步骤

1. 构建：
   ```bash
   npm ci
   npm run build
   ```
2. 运行：
   ```bash
   node .output/server/index.mjs
   ```
3. 推荐配合 PM2：
   ```bash
   pm2 start .output/server/index.mjs --name powerx
   pm2 save
   ```
4. 配置环境变量（`POWERX_BACKEND`、`WS_UPSTREAM`、`SENTRY_DSN` 等）。

---

## 3. Docker 示例

```dockerfile
FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:20-alpine
WORKDIR /app
COPY --from=builder /app/.output ./.output
COPY --from=builder /app/package*.json ./
RUN npm ci --omit=dev
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
```

运行：
```bash
docker build -t powerx .
docker run -p 3000:3000 --env-file .env.production powerx
```

---

## 4. Nginx 反向代理（含 WebSocket/SSE）

```nginx
server {
  listen 80;
  server_name admin.powerx.example.com;

  # HTTP → HTTPS
  return 301 https://$host$request_uri;
}

server {
  listen 443 ssl http2;
  server_name admin.powerx.example.com;

  ssl_certificate /etc/letsencrypt/live/admin/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/admin/privkey.pem;

  # 静态资源缓存
  location ~* \.(js|css|png|jpg|svg|woff2)$ {
    proxy_cache_valid 200 30m;
    proxy_pass http://127.0.0.1:3000;
    add_header Cache-Control "public, max-age=1800";
  }

  # SSR / API
  location / {
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_pass http://127.0.0.1:3000;

    # WebSocket
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 3600s;
  }

  # 后端 API 代理示例
  location /api/ {
    proxy_pass http://backend.internal/api/;
  }
}
```

> SSE 同样受益于 `proxy_read_timeout` 的延长设置。

---

## 5. WebSocket/后端代理注意

- `nuxt.config.ts:55` 中开发代理 `/api/` → `POWERX_BACKEND`，生产环境需要在 Nginx 或 API Gateway 配置相同代理。  
- 确保 WebSocket (`/ws`、`/api/agents/stream/ws`) 在反向代理层开启 `proxy_http_version 1.1` 与 `Upgrade` 头。  
- 若使用 CDN（CloudFront/Cloudflare），需确认支持 WebSocket 并开启。

---

## 6. 缓存策略

- 静态资源：`Cache-Control: public, max-age=31536000, immutable`。  
- API 请求：根据后端策略设置 `max-age` 或 `s-maxage`，敏感数据禁止缓存。  
- SSR 页面：可使用 Nginx `proxy_cache` + 缓存键（区分租户/用户）。

---

## 7. 监控与回滚

- 部署后监控 Node 进程、Nginx 状态、Sentry 错误率。  
- 保留上一版镜像/产物，必要时快速回滚。  
- 在滚动更新前下线旧实例的 WebSocket 连接，避免长连接中断导致异常。

---

## 8. 后续计划

- 整合 Kubernetes 部署模板（Deployment/Service/Ingress）。  
- 提供 Terraform/Helm chart 自动化部署方案。  
- 增加健康检查端点 `/healthz` 与探针。  
- 静态托管场景结合 API Gateway + Lambda@Edge，实现全球加速。
