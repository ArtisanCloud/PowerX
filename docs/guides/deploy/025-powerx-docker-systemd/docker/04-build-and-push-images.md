# 04. 打镜像并推送仓库（可选）

## 1. 目标
产出并推送 3 个镜像：
- `ghcr.io/artisancloud/powerx-backend:<TAG>`
- `ghcr.io/artisancloud/powerx-runner:<TAG>`
- `ghcr.io/artisancloud/powerx-web-admin:<TAG>`

说明：当前仓库未包含业务 `Dockerfile`。默认由 CI/发布仓库执行镜像构建；本手册给出标准构建命令模板，供你在有 Dockerfile 的构建上下文中执行。

## 2. 前置条件
- 已有可用 Docker Build Context（包含 backend/runner/web-admin 的 Dockerfile）。
- 已安装 Docker（建议 24+）和 buildx。
- 已拿到仓库推送权限（GHCR token）。

## 3. 登录镜像仓库

```bash
export REGISTRY=ghcr.io
export ORG=artisancloud
export GHCR_USER='<your-user>'
export GHCR_TOKEN='<your-token>'

echo "$GHCR_TOKEN" | docker login $REGISTRY -u "$GHCR_USER" --password-stdin
```

预期结果：输出 `Login Succeeded`。
失败处理：检查 token 是否有 `write:packages` 权限。

## 4. 设定版本号（Tag）

```bash
export TAG='v2.0.1'
# 建议同时保留 commit tag，便于审计与回滚
export GIT_SHA_TAG="sha-$(git rev-parse --short HEAD)"
```

## 5. 构建并推送镜像（模板）

### 5.1 Backend

```bash
# 将 <BACKEND_DOCKERFILE> 与 <BACKEND_CONTEXT> 替换为你实际路径
docker buildx build \
  -f <BACKEND_DOCKERFILE> \
  -t ghcr.io/artisancloud/powerx-backend:${TAG} \
  -t ghcr.io/artisancloud/powerx-backend:${GIT_SHA_TAG} \
  --push \
  <BACKEND_CONTEXT>
```

### 5.2 Runner

```bash
# 将 <RUNNER_DOCKERFILE> 与 <RUNNER_CONTEXT> 替换为你实际路径
docker buildx build \
  -f <RUNNER_DOCKERFILE> \
  -t ghcr.io/artisancloud/powerx-runner:${TAG} \
  -t ghcr.io/artisancloud/powerx-runner:${GIT_SHA_TAG} \
  --push \
  <RUNNER_CONTEXT>
```

### 5.3 Web Admin

```bash
# 将 <WEB_ADMIN_DOCKERFILE> 与 <WEB_ADMIN_CONTEXT> 替换为你实际路径
docker buildx build \
  -f <WEB_ADMIN_DOCKERFILE> \
  -t ghcr.io/artisancloud/powerx-web-admin:${TAG} \
  -t ghcr.io/artisancloud/powerx-web-admin:${GIT_SHA_TAG} \
  --push \
  <WEB_ADMIN_CONTEXT>
```

## 6. 推送后校验

```bash
docker buildx imagetools inspect ghcr.io/artisancloud/powerx-backend:${TAG}
docker buildx imagetools inspect ghcr.io/artisancloud/powerx-runner:${TAG}
docker buildx imagetools inspect ghcr.io/artisancloud/powerx-web-admin:${TAG}
```

预期结果：3 个镜像 tag 均可查询到 manifest。
失败处理：若镜像不存在，回查 build 日志与仓库权限。

## 7. 输出给部署侧的交付物
- 镜像 tag：`${TAG}`（或 `${GIT_SHA_TAG}`）
- 变更摘要：本次 commit、变更模块、兼容性说明
- 回滚 tag：上一稳定版本 tag
