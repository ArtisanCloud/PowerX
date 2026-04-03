# systemd 部署文档索引（025）

本目录提供 PowerX systemd 生产部署分步手册，按“先准备制品，再部署启动”组织。
默认发布基准为 Git tag（例如 `v2.0.2`），`develop` 仅用于预发布验证。

## 文档列表
- `../00-required-config.md`：部署前必须配置项总清单（强烈建议先核对）。
- `../setup.md`：首次安装引导（`/setup`）完整步骤与排障（Docker/systemd 通用）。
- `01-prepare-artifacts.md`：如何准备 backend/web-admin 发布制品（runner 为可选章节）。
- `02-deploy-config-start.md`：如何安装 service、配置环境、启动服务。
  - 同文包含首次安装引导配置页（`/setup`）的触发条件、接口与排障。
- `03-verify-and-rollback.md`：如何验收、排障与快速回滚。

## 一键打包
- 提供 `make dist`（等价 `make dist-systemd`），输出目录：`dist/systemd/<version>/`
- 常用命令：
  - `export VERSION=v2.0.1`
  - `make dist DIST_VERSION=$VERSION`
  - `make dist DIST_VERSION=$VERSION NPM_INSTALL=0`（跳过 `npm ci`，仅构建）

## 配置规范（可追溯）
- 规则 R1（构建期）：`make dist` 不依赖 `.env` 文件，发布制品以 `backend/etc/config.yaml` 为主配置来源。
- 规则 R2（运行期）：systemd 可选通过 `EnvironmentFile=/etc/powerx/powerx.env` 覆盖少量运行参数；该文件不参与构建产物签名与版本化。
- 规则 R3（审计）：生产变更必须同时记录：
  - 发布版本：`dist/systemd/<version>/manifest.txt`
  - 运行配置：`/opt/powerx/backend/etc/config.yaml`（或软链目标）
  - 环境覆盖（若启用）：`/etc/powerx/powerx.env`
- 规则 R4（优先级）：同一项配置冲突时，进程环境变量优先于 `config.yaml`。

## 对应资产
- systemd 单元文件：`deploy/powerx/systemd/{powerx-backend.service,powerx-runner.service,powerx-web-admin.service}`
- 健康检查脚本：`backend/scripts/ops/deploy-check.sh`
- 软链切换脚本：`backend/scripts/ops/switch-release-systemd.sh`
- API 回滚脚本：`backend/scripts/ops/rollback-release.sh`
