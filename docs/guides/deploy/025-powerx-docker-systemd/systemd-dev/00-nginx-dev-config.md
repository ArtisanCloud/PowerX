# 00. Nginx Dev 站点配置

## 1. 目标
为 dev 域名提供独立入口：
- `https://<dev-domain>/` 转发到 web-admin dev 端口 `3001`
- `https://<dev-domain>/api/` 转发到 backend dev 端口 `8081`
- `https://<dev-domain>/api/ws` 支持 WebSocket
- `https://<dev-domain>/_p/<plugin>/admin/...` 走 web-admin dev 端口 `3001` 的 Nuxt middleware，由 middleware 补齐宿主登录态后再代理到 backend
- `https://<dev-domain>/_p/<plugin>/api/ws` 支持插件 host-mode WebSocket

## 2. 设置变量
```bash
export POWERX_DEV_DOMAIN=dev.powerx.example.com
export POWERX_DEV_CERT_NAME=powerx-dev-cert
export POWERX_DEV_WEB_PORT=3001
export POWERX_DEV_BACKEND_PORT=8081
```

说明：
- `POWERX_DEV_DOMAIN` 替换为实际 dev 域名。
- `POWERX_DEV_CERT_NAME` 替换为 `certbot certificates` 中的证书名称。

## 3. 写入站点配置
创建或覆盖：

```bash
sudo tee /etc/nginx/sites-available/powerx-dev.conf >/dev/null <<'EOF_NGX'
server {
    listen 443 ssl http2;
    server_name __POWERX_DEV_DOMAIN__;

    ssl_certificate /etc/letsencrypt/live/__POWERX_DEV_CERT_NAME__/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/__POWERX_DEV_CERT_NAME__/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;

    client_max_body_size 500m;

    location / {
        proxy_pass http://127.0.0.1:__POWERX_DEV_WEB_PORT__;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    location /api/ws {
        proxy_pass http://127.0.0.1:__POWERX_DEV_BACKEND_PORT__/api/ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    location /api/ {
        client_max_body_size 500m;
        proxy_pass http://127.0.0.1:__POWERX_DEV_BACKEND_PORT__;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    location /api/_nuxt_icon/ {
        proxy_pass http://127.0.0.1:__POWERX_DEV_WEB_PORT__;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location ~ ^/_p/[^/]+/api/ws$ {
        proxy_pass http://127.0.0.1:__POWERX_DEV_BACKEND_PORT__;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}

server {
    listen 80;
    listen [::]:80;
    server_name __POWERX_DEV_DOMAIN__;

    location ^~ /.well-known/acme-challenge/ {
        root /var/www/letsencrypt;
        default_type text/plain;
        try_files $uri =404;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}
EOF_NGX

sudo sed -i \
  -e "s|__POWERX_DEV_DOMAIN__|${POWERX_DEV_DOMAIN}|g" \
  -e "s|__POWERX_DEV_CERT_NAME__|${POWERX_DEV_CERT_NAME}|g" \
  -e "s|__POWERX_DEV_WEB_PORT__|${POWERX_DEV_WEB_PORT}|g" \
  -e "s|__POWERX_DEV_BACKEND_PORT__|${POWERX_DEV_BACKEND_PORT}|g" \
  /etc/nginx/sites-available/powerx-dev.conf
```

## 4. 启用站点
```bash
sudo ln -sfn /etc/nginx/sites-available/powerx-dev.conf /etc/nginx/sites-enabled/powerx-dev.conf
sudo nginx -t
sudo systemctl reload nginx
```

## 5. 验证证书
```bash
sudo certbot certificates
```

预期证书包含：
```text
Identifiers: <prod-domain> <dev-domain>
```

## 6. 验证代理
Dev 服务启动后执行：

```bash
curl -I https://${POWERX_DEV_DOMAIN}
curl -f http://127.0.0.1:8081/api/v1/health
```

插件 Admin 页面应走 web-admin middleware，以便补齐宿主登录态；插件 WS 单独直连 backend：

```bash
curl -I "https://${POWERX_DEV_DOMAIN}/_p/com.powerx.plugins.ai-craft/admin/"
curl -I "https://${POWERX_DEV_DOMAIN}/_p/com.powerx.plugins.ai-craft/api/ws"
```

若插件 Admin 响应头出现 `x-px-proxy-target`，目标必须是 `127.0.0.1:${POWERX_DEV_BACKEND_PORT}`，不能是 prod backend `127.0.0.1:8080`。不要配置普通 `location /_p/` 直连 backend，否则浏览器顶层访问插件 Admin 时不会经过 web-admin 的 Bearer 回填逻辑。
