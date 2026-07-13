下面是**更新后的 README 文档**（在你现有内容基础上新增了 `perm.mk` 相关说明与命令）。直接整体替换你当前的 `make_files/README.md` 即可。如果你的仓库把这些文件放在 `pkg/make_files/`，把文中的路径相应替换一下即可。

---

# Makefile 结构说明

本项目采用模块化的 Makefile 结构，将不同场景的构建任务分离到不同的文件中，便于维护和扩展。

## 📁 文件结构

```
make_files/
├── README.md       # 本说明文件
├── config.mk       # 全局配置和变量定义
├── build.mk        # 构建相关任务
├── test.mk         # 测试相关任务
├── dev.mk          # 开发相关任务
├── clean.mk        # 清理相关任务
├── docker.mk       # Docker 相关任务
├── database.mk     # 数据库相关任务
├── mcp.mk          # MCP 服务相关任务
└── perm.mk         # OpenAPI & 权限同步（Swagger 生成 / 最小 OpenAPI / 权限目录同步）
```

> 注：若你的仓库使用 `pkg/make_files/` 作为目录，请将上述路径替换为 `pkg/make_files/`。

## 🎯 各文件职责

### config.mk - 全局配置

* 项目基本信息（名称、版本等）
* 构建目录和文件路径
* Go 编译参数
* 颜色输出定义

### build.mk - 构建任务

* `make build` - 构建所有组件
* `make build-agent` - 构建 agent 组件
* `make build-api` - 构建 API 组件
* `make build-all` - 完整构建

### test.mk - 测试任务

* `make test` - 运行所有测试
* `make test-unit` - 单元测试
* `make test-agent` - Agent 功能测试
* `make test-api` - API 测试
* `make test-eino` - Eino 框架测试

### dev.mk - 开发任务

* `make dev` - 启动开发服务器
* `make dev-watch` - 监听文件变化自动重启
* `make fmt` - 代码格式化
* `make lint` - 代码检查
* `make deps` - 安装依赖

### clean.mk - 清理任务

* `make clean` - 清理构建文件
* `make clean-all` - 完全清理
* `make clean-temp` - 清理临时文件

### docker.mk - Docker 任务

* `make docker-build` - 构建 Docker 镜像
* `make docker-run` - 运行容器
* `make docker-push` - 推送镜像
* `make docker-clean` - 清理 Docker 资源

### database.mk - 数据库任务

* `make db-migrate` - 数据库迁移
* `make db-seed` - 只执行 CoreX / 数据库基础种子数据
* `make capability-seed` - 只同步 `backend/config/platform_capabilities/*.yaml` 到 Capability Registry
* `make seed` - 执行 `db-seed` 后再执行 `capability-seed`
* `make db-reset` - 重置数据库

### mcp.mk - MCP 服务任务

* `make -f make_files/mcp.mk mcp-run` - 运行 MCP 服务
* `make -f make_files/mcp.mk mcp-dev` - 开发模式运行 MCP 服务
* `make -f make_files/mcp.mk mcp-build` - 构建 MCP 服务
* `make -f make_files/mcp.mk mcp-test` - 运行 MCP 测试
* `make -f make_files/mcp.mk mcp-clean` - 清理 MCP 构建文件

### perm.mk - OpenAPI & 权限同步任务

面向 **Swagger 文档生成**与**权限目录同步**的自动化脚本集合，覆盖两种来源：

1. **标准 Swagger 流程**：从注解生成 `./backend/api/openapi/swagger.json` → 生成/同步权限；
2. **最小 OpenAPI 流程**：运行中的服务挂载 `/openapi.min.json` → 生成/同步权限（无需注解，先上车）。

**主要目标：**

* `make tools.swag`：安装指定版本的 `swag` CLI（默认 `v1.16.3`）
* `make deps.swag`：对齐项目中 `github.com/swaggo/swag` 等依赖版本，避免 LeftDelim/RightDelim 不兼容
* `make swagger.gen`：从入口文件（默认 `cmd/app/main.go`）生成到 `./docs`
* `make swagger.clean`：清理 `./docs`
* `make permgen.print`：基于 `./backend/api/openapi/swagger.json` 生成 **权限同步 dry-run 载荷**
* `make permgen.apply`：基于 `./backend/api/openapi/swagger.json` **落库**（调用内部 `SyncPermissions`）
* `make permgen.min.print`：基于 `http://localhost:PORT/openapi.min.json` **dry-run**
* `make permgen.min.apply`：基于最小 OpenAPI **落库**

**可覆盖变量（命令行传参覆盖）**：

* `APP_MAIN`（默认 `cmd/app/main.go`）
* `SWAG_VERSION`（默认 `v1.16.3`）
* `DOCS_DIR`（默认 `./docs`）
* `PORT`（默认 `8077`，用于最小 OpenAPI）
* `SOURCE`（默认 `core`，权限来源标识）
* `INTRODUCED`（默认 `v1.0.0`，版本号写入权限条目）

**根 Makefile 引入：**

```makefile
include make_files/perm.mk
# 若使用 pkg 目录结构：
# include pkg/make_files/perm.mk
```

## 🚀 使用方法

### 常用命令

```bash
# 查看所有可用命令
make help

# 开发环境启动
make dev

# 运行测试
make test

# 构建项目
make build

# 清理项目
make clean

# MCP 服务相关命令
make -f make_files/mcp.mk mcp-dev     # 开发模式启动 MCP 服务
make -f make_files/mcp.mk mcp-run     # 生产模式启动 MCP 服务
make -f make_files/mcp.mk mcp-test    # 运行 MCP 测试
```

### OpenAPI / 权限同步常用命令

**标准 Swagger（有注解）**

```bash
make deps.swag tools.swag             # 安装并对齐 swag 版本
make swagger.gen                      # 生成 ./backend/api/openapi/swagger.json
make permgen.print SOURCE=core INTRODUCED=v1.0.0     # 预演差异
make permgen.apply SOURCE=core INTRODUCED=v1.0.0     # 同步落库
```

**最小 OpenAPI（无注解，服务需挂 /openapi.min.json）**

```bash
# 启动服务并确认 http://localhost:8077/openapi.min.json 可访问
make permgen.min.print PORT=8077 SOURCE=core INTRODUCED=v1.0.0
make permgen.min.apply PORT=8077 SOURCE=core INTRODUCED=v1.0.0
```

> 提示：`SOURCE` 建议用 `core` 或插件 ID；`INTRODUCED` 写入版本号便于审计。
> 最小 OpenAPI 的 Swagger UI 可通过 `ginSwagger.URL("/openapi.min.json")` 指向渲染。

### 开发流程

```bash
# 1. 安装依赖
make deps

# 2. 代码格式化和检查
make fmt
make lint

# 3. 运行测试
make test

# 4. 启动开发服务器
make dev
```

### MCP 服务开发流程

```bash
# 1. 查看 MCP 服务帮助
make -f make_files/mcp.mk mcp-help

# 2. 安装 MCP 依赖
make -f make_files/mcp.mk mcp-install

# 3. 检查 MCP 服务配置
make -f make_files/mcp.mk mcp-check

# 4. 开发模式启动 MCP 服务
make -f make_files/mcp.mk mcp-dev

# 5. 测试 MCP 工具调用
make -f make_files/mcp.mk mcp-test-tool

# 6. 健康检查
make -f make_files/mcp.mk mcp-health
```

### 密钥管理

# 1) 生成并校验（写到 .env.wrap）

make secrets

# 2) 本地终端临时导入

eval "$(make export)"

# 3) Docker Compose（生成 .env）

make compose-env

# 4) Kubernetes Secret（生成 YAML）

make k8s-secret   # 输出到 build/k8s/wrap-master-key-secret.yaml

# 5) systemd 环境片段

make systemd-env  # 然后按提示 cp 到 /etc/systemd/system/<unit>.d/

### 部署流程

```bash
# 1. 构建项目
make build

# 2. 运行测试
make test

# 3. 构建 Docker 镜像
make docker-build

# 4. 推送镜像
make docker-push
```

## 🔧 自定义配置

如需修改配置，请编辑 `make_files/config.mk` 文件：

```makefile
# 修改项目名称
PROJECT_NAME := your-project-name

# 修改构建目录
BUILD_DIR := your-build-dir

# 修改 Go 版本
GO_VERSION := 1.21
```

## 📝 添加新任务

要添加新的构建任务，请在相应的 `.mk` 文件中添加：

```makefile
# 在 build.mk 中添加新的构建任务
build-new-component:
 @echo "$(CYAN)构建新组件...$(NC)"
 @go build -o $(BUILD_DIR)/new-component ./backend/cmd/new-component
 @echo "$(GREEN)✅ 新组件构建完成$(NC)"
```

## 🎨 颜色输出

所有任务都支持彩色输出，使用以下颜色：

* 🔵 蓝色 (CYAN) - 信息提示
* 🟢 绿色 (GREEN) - 成功信息
* 🟡 黄色 (YELLOW) - 警告信息
* 🔴 红色 (RED) - 错误信息

## 🤝 贡献指南

1. 保持每个 `.mk` 文件的职责单一
2. 添加适当的颜色输出和进度提示
3. 为新任务添加帮助信息
4. 确保任务的幂等性（可重复执行）
5. 添加必要的错误处理

## 📚 参考资料

* [GNU Make 官方文档](https://www.gnu.org/software/make/manual/)
* [Go 构建最佳实践](https://golang.org/doc/code.html)
* [Docker 最佳实践](https://docs.docker.com/develop/dev-best-practices/)
* [swaggo/swag](https://github.com/swaggo/swag)（Swagger 生成器）
* [Swagger UI 参数：ginSwagger.URL](https://github.com/swaggo/gin-swagger)

---

需要我把 `perm.mk` 的文件也一并落到你的仓库路径里吗？我可以给你一键拷贝版。
