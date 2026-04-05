# 00. Nginx 安装与配置（systemd）

## 1. 目标
为 PowerX 提供统一入口域名与反向代理：
- `/<...>` 转发到 web-admin（默认 `3000`）
- `/api/` 与 `/grpc-gateway/` 转发到 backend（默认 `8080`）
- `/api/ws` 支持 WebSocket 升级

## 2. 安装 Nginx（Ubuntu/Debian）
```bash
sudo apt-get update
sudo apt-get install -y nginx
sudo systemctl enable nginx
sudo systemctl start nginx
sudo systemctl status nginx --no-pager
```

## 3. 创建站点配置

先设置变量（示例）：
```bash
export POWERX_SERVER_NAME=powerx.example.com
export POWERX_WEB_ADMIN_PORT=3000
export POWERX_BACKEND_PORT=8080
```

写入配置：
```bash
sudo tee /etc/nginx/sites-available/powerx.conf >/dev/null <<'EOF_NGX'
server {
    listen 80;
    server_name powerx.example.com;

    client_max_body_size 50m;

    # WebSocket
    location /api/ws {
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:8080;
    }

    # Backend API
    # 注意：Nuxt Icon 接口必须先于 /api/ 命中并转发到 web-admin
    location /api/_nuxt_icon/ {
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:3000;
    }

    # Backend API
    location /api/ {
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:8080;
    }

    # 如有网关路径可按需保留
    location /grpc-gateway/ {
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:8080;
    }

    # Web Admin
    location / {
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:3000;
    }
}
EOF_NGX
```

说明：
- 上面默认写死 `3000/8080`，与你的 systemd 配置不一致时请改成实际端口。
- 若你已经有 HTTPS 入口（例如云 LB），可先保留 80 端口配置。

## 4. 启用站点并重载
```bash
sudo ln -sfn /etc/nginx/sites-available/powerx.conf /etc/nginx/sites-enabled/powerx.conf
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t
sudo systemctl reload nginx
```

## 5. 验证
```bash
curl -I http://127.0.0.1/
curl -f http://127.0.0.1/api/v1/health
```

预期：
- `/` 返回 web-admin 页面响应
- `/api/v1/health` 返回 200

## 6. 可选：启用 HTTPS（Let\'s Encrypt）
```bash
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d powerx.example.com
```

续期检查：
```bash
sudo certbot renew --dry-run
```

## 7. 双域名模板（可选）

适用：你希望管理端与 API 分离域名。
- 管理端：`powerx.example.com` -> `127.0.0.1:3000`
- API：`api.powerx.example.com` -> `127.0.0.1:8080`

```nginx
server {
    listen 80;
    server_name powerx.example.com;

    client_max_body_size 50m;

    location / {
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:3000;
    }
}

server {
    listen 80;
    server_name api.powerx.example.com;

    client_max_body_size 50m;

    location /api/ws {
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:8080;
    }

    # 注意：单独保留 Nuxt Icon 路由到 web-admin，避免被 /api/ 误转发到 backend
    location /api/_nuxt_icon/ {
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:3000;
    }

    location /api/ {
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:8080;
    }

    location /grpc-gateway/ {
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:8080;
    }
}
```

双域名验证：
```bash
curl -I http://powerx.example.com/
curl -f http://api.powerx.example.com/api/v1/health
```
