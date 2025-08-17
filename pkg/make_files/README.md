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
└── mcp.mk          # MCP 服务相关任务
```

## 🎯 各文件职责

### config.mk - 全局配置
- 项目基本信息（名称、版本等）
- 构建目录和文件路径
- Go 编译参数
- 颜色输出定义

### build.mk - 构建任务
- `make build` - 构建所有组件
- `make build-agent` - 构建 agent 组件
- `make build-api` - 构建 API 组件
- `make build-all` - 完整构建

### test.mk - 测试任务
- `make test` - 运行所有测试
- `make test-unit` - 单元测试
- `make test-agent` - Agent 功能测试
- `make test-api` - API 测试
- `make test-eino` - Eino 框架测试

### dev.mk - 开发任务
- `make dev` - 启动开发服务器
- `make dev-watch` - 监听文件变化自动重启
- `make fmt` - 代码格式化
- `make lint` - 代码检查
- `make deps` - 安装依赖

### clean.mk - 清理任务
- `make clean` - 清理构建文件
- `make clean-all` - 完全清理
- `make clean-temp` - 清理临时文件

### docker.mk - Docker 任务
- `make docker-build` - 构建 Docker 镜像
- `make docker-run` - 运行容器
- `make docker-push` - 推送镜像
- `make docker-clean` - 清理 Docker 资源

### database.mk - 数据库任务
- `make db-migrate` - 数据库迁移
- `make db-seed` - 数据库种子数据
- `make db-reset` - 重置数据库

### mcp.mk - MCP 服务任务
- `make -f make_files/mcp.mk mcp-run` - 运行 MCP 服务
- `make -f make_files/mcp.mk mcp-dev` - 开发模式运行 MCP 服务
- `make -f make_files/mcp.mk mcp-build` - 构建 MCP 服务
- `make -f make_files/mcp.mk mcp-test` - 运行 MCP 测试
- `make -f make_files/mcp.mk mcp-clean` - 清理 MCP 构建文件

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
	@go build -o $(BUILD_DIR)/new-component ./cmd/new-component
	@echo "$(GREEN)✅ 新组件构建完成$(NC)"
```

## 🎨 颜色输出

所有任务都支持彩色输出，使用以下颜色：
- 🔵 蓝色 (CYAN) - 信息提示
- 🟢 绿色 (GREEN) - 成功信息
- 🟡 黄色 (YELLOW) - 警告信息
- 🔴 红色 (RED) - 错误信息

## 🤝 贡献指南

1. 保持每个 `.mk` 文件的职责单一
2. 添加适当的颜色输出和进度提示
3. 为新任务添加帮助信息
4. 确保任务的幂等性（可重复执行）
5. 添加必要的错误处理

## 📚 参考资料

- [GNU Make 官方文档](https://www.gnu.org/software/make/manual/)
- [Go 构建最佳实践](https://golang.org/doc/code.html)
- [Docker 最佳实践](https://docs.docker.com/develop/dev-best-practices/)