# PowerX CI 自动化策略

本文档定义 PowerX 统一 CI 流程的设计准则，确保后台（Go）与 Web Admin（Nuxt/Vue）代码在提交/合并前都经过一致、可重复的验证。

## 目标

1. **统一入口**：所有仓库子模块在同一条 GitHub Actions workflow 中完成构建、检查与测试。
2. **最小回归窗口**：PR 必须在 CI 通过后才能合并，master/main 上保持“永远可部署”状态。
3. **可本地复现**：CI 中的每一步对应清晰的 `make` / `pnpm` / `scripts/ci` 命令，开发者在本地即可复跑。

## 总体流程

| 阶段 | 目标 | 主要命令 |
| --- | --- | --- |
| 1. 初始化 | 设置 Go/Node 版本、缓存依赖 | `setup-go@v5`, `setup-node@v4`, `pnpm/action-setup@v2` |
| 2. 后端静态检查 | 运行 linters、UUID 扫描、生成代码差异 | `make lint`, `buf lint` |
| 3. 后端测试 | 运行所有 Go 单元/集成测试 | `make test-backend`（内部调用 `go test ./...` + 必要的 integration flag） |
| 4. 前端检查 | 安装 pnpm 依赖、运行 lint/type-check | `pnpm install --frozen-lockfile`, `pnpm lint`, `pnpm typecheck` |
| 5. 前端测试 | 运行 Vitest + 关键 Playwright 套件 | `pnpm vitest run --runInBand`, `pnpm playwright test --config=e2e` |
| 6. 结果上报 | 汇总所有步骤的 JUnit/coverage 报告，必要时上传 artifacts | GitHub Actions `actions/upload-artifact` |

> **推荐触发器**：`pull_request`（所有 PR 必须通过）、`push`（保护 main 分支）、`workflow_dispatch`（手动触发全量验证）。

## 后端阶段设计

- **环境**：Go 1.24，设置 `GOMODCACHE`/`GOCACHE` 对应的 GitHub Actions 缓存键，减少安装时间。
- **静态检查**  
- `make lint`：统一包裹 `golangci-lint`、`buf lint`、`go vet`。  
  - `scripts/ci/check-tenant-uuid-canonical.sh`：扫描 `uuid.Parse(tenantUUID` 等违规写法。
- **单元/集成测试**  
  - `make test-backend` 负责 `go test ./...`，并给指定模块（Event Fabric、Workflow、Integration Gateway 等）注入所需的 SQLite/MinIO/Redis 测试容器（推荐用 `services:` + docker compose）。  
  - 如需加速，可按包输出 JUnit：`gotestsum --format short-verbose --junitfile=junit/go-tests.xml`.

## 前端阶段设计

- **环境**：Node 20 + pnpm（锁定 pnpm 版本，启用 pnpm store cache）。
- **依赖安装**：`pnpm install --frozen-lockfile`，并缓存 `~/.pnpm-store`.
- **质量检查**：  
  - `pnpm lint`：统一运行 ESLint。  
  - `pnpm typecheck`：调用 `nuxt typecheck`，保证 TS 正确。
- **单元测试**：`pnpm vitest run --runInBand --coverage`，输出 `junit/vitest.xml` + `coverage/vitest/`.
- **端到端测试（可选/按需）**：对关键流程启用 `pnpm playwright test --config=tests/e2e/playwright.config.ts`，使用 GitHub Actions 提供的 Chromium/WebKit runners。

## GitHub Actions 示例片段

```yaml
name: powerx-ci

on:
  pull_request:
  push:
    branches: [main]

jobs:
  backend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - run: make lint
      - run: make test-backend

  web-admin:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web-admin
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
      - uses: pnpm/action-setup@v2
        with:
          version: 8
      - run: pnpm install --frozen-lockfile
      - run: pnpm lint
      - run: pnpm typecheck
      - run: pnpm vitest run --runInBand
```

（可根据需要继续添加 Playwright 阶段、artifact 上传与矩阵构建等。）

## 本地复现

- 后端开发者：`make lint && make test-backend`.
- 前端开发者：`cd web-admin && pnpm install && pnpm lint && pnpm vitest`.

将上述命令写入 `Makefile`/`package.json scripts`，保证“CI 做的每一步都能在本机跑”。  
后续扩展（例如容器构建扫描、SCA、镜像推送）也建议按照同样的分阶段结构追加，保持 CI 任务清晰可维护。
