# 01. 准备发布制品（systemd）

## 1. 目标
准备可在目标机直接运行的三类制品：
- backend 可执行文件：`powerx`
- backend 运行配置：`backend/etc/config.yaml`
- runner Node 产物：`dist/main.js`
- web-admin Nuxt 产物：`.output/server/index.mjs`
- 环境变量模板：
  - backend/runtime：`config/powerx.env`（来源：`backend/.env.example`）
  - web-admin：`config/web-admin.env`（来源：`web-admin/.env.example`）

同时支持两种方式：
- 手工步骤（逐条执行，便于排错）
- `make dist` 一键打包（便于流水线/重复发布）

## 2. 前置条件
- 构建机已安装 Go 1.24、Node 20、npm。
- 已切到目标发布分支/commit。
- 已规划发布目录：`/opt/powerx/releases/<version>/`。
- 以下命令默认在仓库根目录执行：`/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX`。
- Go 模块在 `backend/go.mod`，因此构建 backend 时必须先 `cd backend`（或使用 `go -C backend ...`）。
- 部署前请先核对必须配置项：`../00-required-config.md`。

## 3. 构建 backend

```bash
export VERSION=v2.0.1
mkdir -p dist/systemd/${VERSION}/backend
cd backend
go build -o ../dist/systemd/${VERSION}/backend/powerx ./cmd/app
cd ..
```

预期结果：生成 `dist/systemd/${VERSION}/backend/powerx`。
失败处理：先执行 `go mod tidy` 并检查 Go 版本。

补充说明：
- `cmd/app` 是后端服务入口；`cmd/px` 是 CLI 工具入口，不用于拉起 HTTP/gRPC 服务。
- 后端默认读取 `etc/config.yaml`，因此发布包必须包含 `backend/etc/config.yaml`。

## 4. 构建 web-admin

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

## 5. 复制配置、systemd 单元与 env 模板到 dist

```bash
mkdir -p dist/systemd/${VERSION}/backend/etc
cp -R backend/etc/* dist/systemd/${VERSION}/backend/etc/

mkdir -p dist/systemd/${VERSION}/systemd dist/systemd/${VERSION}/config
cp deploy/powerx/systemd/*.service dist/systemd/${VERSION}/systemd/
cp backend/.env.example dist/systemd/${VERSION}/config/powerx.env
cp web-admin/.env.example dist/systemd/${VERSION}/config/web-admin.env
```

预期结果：`dist/systemd/${VERSION}` 包含可部署所需二进制、配置和 service 文件。
失败处理：校验文件权限与目录结构。

## 6. 调整 dist 中配置（发布前）

- 编辑 `dist/systemd/${VERSION}/backend/etc/config.yaml`：数据库、缓存、对象存储、认证等。
- 编辑 `dist/systemd/${VERSION}/config/powerx.env`（部署时复制到 `/etc/powerx/powerx.env`）。
- `dist/systemd/${VERSION}/config/web-admin.env` 用于 web-admin 专项变量参考（如需拆分独立 EnvironmentFile 可基于此扩展）。
- 兼容产物：同时保留 `.example` 文件，便于历史脚本继续使用。

## 6.1 本地手动启动验证（macOS/Linux 通用）

在 systemd 部署前，可先用 dist 产物里的 env 做手动验证：

```bash
# 终端1：backend
set -a
source dist/systemd/${VERSION}/config/powerx.env
set +a
cd backend && go run ./cmd/app
```

```bash
# 终端2：web-admin
set -a
source dist/systemd/${VERSION}/config/powerx.env
set +a
cd web-admin && PORT=${POWERX_WEB_ADMIN_PORT} UPSTREAM=http://127.0.0.1:${POWERX_BACKEND_PORT} npm run dev
```

预期结果：backend 监听 `POWERX_BACKEND_PORT`，web-admin 监听 `POWERX_WEB_ADMIN_PORT`。

## 6.2 直接从 dist 制品启动（不依赖源码构建）

当你要验证发布包本身（`dist/systemd/${VERSION}`）是否可运行时，使用以下方式：

```bash
# 终端1：backend（二进制）
cd /private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX
set -a
source dist/systemd/${VERSION}/config/powerx.env
set +a
./dist/systemd/${VERSION}/backend/powerx
```

```bash
# 终端2：web-admin（Nuxt server 产物）
cd /private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX
set -a
source dist/systemd/${VERSION}/config/powerx.env
set +a
PORT=${POWERX_WEB_ADMIN_PORT} \
UPSTREAM=http://127.0.0.1:${POWERX_BACKEND_PORT} \
node dist/systemd/${VERSION}/web-admin/.output/server/index.mjs
```

验证命令：

```bash
curl -f http://127.0.0.1:${POWERX_BACKEND_PORT}/api/v1/health
open http://127.0.0.1:${POWERX_WEB_ADMIN_PORT}
```

## 7. 发布到目标机目录（示例）

```bash
sudo mkdir -p /opt/powerx/releases/${VERSION}
sudo cp -R dist/systemd/${VERSION}/backend /opt/powerx/releases/${VERSION}/
sudo cp -R dist/systemd/${VERSION}/web-admin /opt/powerx/releases/${VERSION}/

sudo ln -sfn /opt/powerx/releases/${VERSION}/backend /opt/powerx/backend
sudo ln -sfn /opt/powerx/releases/${VERSION}/web-admin /opt/powerx/web-admin
```

预期结果：`/opt/powerx/{backend,web-admin}` 指向当前发布版本。

## 8. （可选）构建并发布 runner

仅当你的分支/版本包含 `backend/runner` 目录时执行本节。

```bash
cd backend/runner
npm ci
npm run build
cd ../..
mkdir -p dist/systemd/${VERSION}/runner
cp -R backend/runner/dist dist/systemd/${VERSION}/runner/

sudo cp -R dist/systemd/${VERSION}/runner /opt/powerx/releases/${VERSION}/
sudo ln -sfn /opt/powerx/releases/${VERSION}/runner /opt/powerx/runner
```

预期结果：生成并发布 `dist/systemd/${VERSION}/runner/dist/main.js`。
失败处理：检查 Node 版本与依赖安装日志，以及 `backend/runner` 目录是否存在。

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
├── web-admin/.output/...
├── config/powerx.env
├── config/web-admin.env
├── config/powerx.env.example
├── config/web-admin.env.example
├── systemd/
│   ├── powerx-backend.service
│   ├── powerx-runner.service
│   └── powerx-web-admin.service
└── manifest.txt
```

说明：若源码包含 `backend/runner` 且你启用了 runner 打包流程，产物中会额外出现 `runner/dist/...`。
