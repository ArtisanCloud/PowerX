# 01. 准备发布制品（systemd）

## 1. 目标
准备可在目标机直接运行的三类制品：
- backend 可执行文件：`powerx`
- backend 运行配置：`backend/etc/config.yaml`
- runner Node 产物：`dist/main.js`
- web-admin Nuxt 产物：`.output/server/index.mjs`
- 环境变量模板：`config/powerx.env.example`

同时支持两种方式：
- 手工步骤（逐条执行，便于排错）
- `make dist` 一键打包（便于流水线/重复发布）

## 2. 前置条件
- 构建机已安装 Go 1.24、Node 20、npm。
- 已切到目标发布分支/commit。
- 已规划发布目录：`/opt/powerx/releases/<version>/`。

## 3. 构建 backend

```bash
export VERSION=v2.0.1
mkdir -p dist/systemd/${VERSION}/backend
go build -o dist/systemd/${VERSION}/backend/powerx ./backend/cmd/app
```

预期结果：生成 `dist/systemd/${VERSION}/backend/powerx`。
失败处理：先执行 `go mod tidy` 并检查 Go 版本。

补充说明：
- `cmd/app` 是后端服务入口；`cmd/px` 是 CLI 工具入口，不用于拉起 HTTP/gRPC 服务。
- 后端默认读取 `etc/config.yaml`，因此发布包必须包含 `backend/etc/config.yaml`。

## 4. 构建 runner

```bash
cd backend/runner
npm ci
npm run build
cd ../..
mkdir -p dist/systemd/${VERSION}/runner
cp -R backend/runner/dist dist/systemd/${VERSION}/runner/
```

预期结果：生成 `dist/systemd/${VERSION}/runner/dist/main.js`。
失败处理：检查 Node 版本与依赖安装日志。

## 5. 构建 web-admin

```bash
cd web-admin
npm ci
npm run build
cd ..
mkdir -p dist/systemd/${VERSION}/web-admin
cp -R web-admin/.output dist/systemd/${VERSION}/web-admin/
```

预期结果：生成 `dist/systemd/${VERSION}/web-admin/.output/server/index.mjs`。
失败处理：检查前端环境变量与 Nuxt 构建错误。

## 6. 复制配置与 systemd 单元到 dist

```bash
mkdir -p dist/systemd/${VERSION}/backend/etc
cp -R backend/etc/* dist/systemd/${VERSION}/backend/etc/

mkdir -p dist/systemd/${VERSION}/systemd dist/systemd/${VERSION}/config
cp deploy/powerx/systemd/*.service dist/systemd/${VERSION}/systemd/
cat > dist/systemd/${VERSION}/config/powerx.env.example <<'ENV'
POWERX_ENV=prod
POWERX_MODE=systemd
DATABASE_DSN=postgres://powerx:powerx@127.0.0.1:5432/powerx?sslmode=disable
REDIS_ADDR=127.0.0.1:6379
ENV
```

预期结果：`dist/systemd/${VERSION}` 包含可部署所需二进制、配置和 service 文件。
失败处理：校验文件权限与目录结构。

## 7. 调整 dist 中配置（发布前）

- 编辑 `dist/systemd/${VERSION}/backend/etc/config.yaml`：数据库、缓存、对象存储、认证等。
- 编辑 `dist/systemd/${VERSION}/config/powerx.env.example` 并重命名为发布环境文件（部署时复制到 `/etc/powerx/powerx.env`）。

## 8. 发布到目标机目录（示例）

```bash
sudo mkdir -p /opt/powerx/releases/${VERSION}
sudo cp -R dist/systemd/${VERSION}/backend /opt/powerx/releases/${VERSION}/
sudo cp -R dist/systemd/${VERSION}/runner /opt/powerx/releases/${VERSION}/
sudo cp -R dist/systemd/${VERSION}/web-admin /opt/powerx/releases/${VERSION}/

sudo ln -sfn /opt/powerx/releases/${VERSION}/backend /opt/powerx/backend
sudo ln -sfn /opt/powerx/releases/${VERSION}/runner /opt/powerx/runner
sudo ln -sfn /opt/powerx/releases/${VERSION}/web-admin /opt/powerx/web-admin
```

预期结果：`/opt/powerx/{backend,runner,web-admin}` 指向当前发布版本。

关于 `dist`：
- 当前仓库已提供 `make dist`（`make dist-systemd`）用于 systemd 发布打包。
- 现有 Makefile 中 `build` 相关条目仍是旧模板，不匹配当前 `backend/cmd/*` 实际结构，不建议直接使用。

## 9. 一键打包（推荐）

新增打包目标：
- `make dist`（等价 `make dist-systemd`）
- 输出目录：`dist/systemd/<DIST_VERSION>/`

示例：

```bash
# 全量（包含 npm ci）
make dist DIST_VERSION=v2.0.1

# 跳过 npm ci（适合已预装依赖）
make dist DIST_VERSION=v2.0.1 NPM_INSTALL=0
```

产物结构示例：

```text
dist/systemd/v2.0.1/
├── backend/
│   ├── powerx
│   └── etc/config.yaml
├── runner/dist/...
├── web-admin/.output/...
├── config/powerx.env.example
├── systemd/
│   ├── powerx-backend.service
│   ├── powerx-runner.service
│   └── powerx-web-admin.service
└── manifest.txt
```
