# Docker 化与生产部署指南

> 提供将 PowerX Web Admin 部署在 Docker 环境（单机或 K8s）的推荐实践，涵盖镜像构建、环境注入、日志/健康检查与弹性扩缩。

---

## 1. 镜像构建

### 1.1 多阶段 Dockerfile

```dockerfile
FROM node:20-alpine AS deps
WORKDIR /app
COPY package*.json ./
RUN npm ci

FROM node:20-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
COPY --from=builder /app/.output ./.output
COPY --from=deps /app/node_modules ./node_modules
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
```

- 使用 `npm ci` 确保锁定包版本。  
- 构建阶段尽量少 COPY，避免将 `.nuxt` 等缓存带入最终镜像。  
- 若使用 `pnpm`/`yarn`，可根据需要调整。

### 1.2 镜像裁剪

- 使用 `apk add --no-cache libc6-compat` 解决 glibc 依赖。  
- 使用 `docker build --build-arg COMMIT_SHA=$(git rev-parse HEAD)` 写入版本信息。  
- 最终镜像建议 < 300MB，可使用 `node:20-slim` + 自行安装依赖。

---

## 2. 环境变量与配置

- 运行容器时注入 `.env`：  
  ```bash
  docker run --env-file .env.production -p 3000:3000 powerx
  ```
- 对于敏感配置（API Key、Sentry DSN），使用容器平台 Secret（Docker Swarm secrets、Kubernetes Secret）。  
- 参考 `Env_Variables_Schema.md` 保持环境变量一致。

---

## 3. 健康检查

- 在 Nuxt 内新增 `/healthz` API（或使用 Nitro 内置）返回 `{ ok: true }`。  
- Dockerfile：  
  ```dockerfile
  HEALTHCHECK --interval=30s --timeout=3s --start-period=60s \
    CMD wget -qO- http://127.0.0.1:3000/healthz || exit 1
  ```
- K8s:
  ```yaml
  livenessProbe:
    httpGet: { path: /healthz, port: 3000 }
    initialDelaySeconds: 30
    periodSeconds: 30
  readinessProbe:
    httpGet: { path: /healthz, port: 3000 }
    initialDelaySeconds: 10
    periodSeconds: 15
  ```

---

## 4. 日志与监控

- 默认输出 `stdout`/`stderr`，由容器平台收集（CloudWatch、ELK、Loki）。  
- 格式化日志：可在 Nuxt 插件中注入 `console` 包装器或使用 `pino`.  
- 结合 Sentry/Prometheus 监控：部署 `Sentry_Logging_and_Traces.md` 中的方案。  
- 如需访问日志文件，建议挂载卷或使用集中日志系统。

---

## 5. 横向扩展

- 使用多副本时需确保无状态，前端只依赖后端 API。  
- WebSocket/SSE 需通过负载均衡器开启粘性会话（或使用共享会话存储）。  
- Kubernetes 示例：
  ```yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata: { name: powerx }
  spec:
    replicas: 3
    strategy:
      type: RollingUpdate
      rollingUpdate:
        maxSurge: 1
        maxUnavailable: 0
    template:
      spec:
        containers:
          - name: app
            image: registry.example.com/powerx:latest
            ports: [{ containerPort: 3000 }]
            envFrom:
              - secretRef: { name: powerx-env }
              - configMapRef: { name: powerx-config }
  ```

---

## 6. CI/CD 集成

- 在 `CI_CD_Pipeline_for_Nuxt.md` 构建后执行：
  ```bash
  docker build -t registry.example.com/powerx:${GIT_SHA} .
  docker push registry.example.com/powerx:${GIT_SHA}
  ```
- 使用 GitHub Actions `docker/login-action`、`docker/build-push-action`。  
- 部署阶段：更新 K8s Deployment 或 Docker Swarm Stack，等待滚动升级完成。

---

## 7. 回滚策略

- 保留上一版镜像 Tag：`registry.example.com/powerx:rollback`.  
- Kubernetes：`kubectl rollout undo deployment/powerx`.  
- 单机：`docker run` 启动旧版本，同时关停异常容器。

---

## 8. 安全与合规

- 使用非 Root 运行：  
  ```dockerfile
  RUN addgroup -S nuxt && adduser -S nuxt -G nuxt
  USER nuxt
  ```
- 容器镜像定期扫描（Trivy、Grype）。  
- 禁止在镜像中包含 `.env`、证书等敏感文件。

---

## 9. 后续计划

- 构建 Helm Chart / Terraform 模块统一部署。  
- 加入自动扩缩（HPA based on CPU/memory/requests）。  
- 将镜像构建任务集成到 Release Pipeline，使用 Git tag 触发。  
- 建立 `runbook` 处理常见故障（高负载、WS 超时、磁盘满等）。
