# MCP 服务相关命令
# 使用方法: make -f make_files/mcp.mk <target>

.PHONY: mcp-help mcp-build mcp-run mcp-dev mcp-test mcp-clean mcp-install mcp-check

# 默认目标
mcp-help:
	@echo "CoreX MCP 服务管理命令:"
	@echo "  mcp-build     - 编译 MCP 服务"
	@echo "  mcp-run       - 运行 MCP 服务 (stdio 模式)"
	@echo "  mcp-dev       - 开发模式运行 MCP 服务"
	@echo "  mcp-test      - 运行 MCP 服务测试"
	@echo "  mcp-clean     - 清理 MCP 编译文件"
	@echo "  mcp-install   - 安装 MCP 服务依赖"
	@echo "  mcp-check     - 检查 MCP 服务状态"
	@echo ""
	@echo "注意: MCP 服务使用 stdio 通信，适用于 Agent 插件和 CLI 工具"
	@echo ""
	@echo "示例:"
	@echo "  make -f make_files/mcp.mk mcp-run"
	@echo "  make -f make_files/mcp.mk mcp-dev"

# MCP 服务配置
MCP_BINARY_NAME := corex-mcp
MCP_MAIN_PATH := ./mcp/cmd/main.go
MCP_BUILD_DIR := ./build
MCP_CONFIG_PATH := ./config/example.yaml

# 编译 MCP 服务
mcp-build:
	@echo "🔨 编译 CoreX MCP 服务..."
	@mkdir -p $(MCP_BUILD_DIR)
	@go build -o $(MCP_BUILD_DIR)/$(MCP_BINARY_NAME) $(MCP_MAIN_PATH)
	@echo "✅ MCP 服务编译完成: $(MCP_BUILD_DIR)/$(MCP_BINARY_NAME)"

# 运行 MCP 服务
mcp-run: mcp-build
	@echo "🚀 启动 CoreX MCP 服务 (stdio 模式)..."
	@echo "📋 配置文件: $(MCP_CONFIG_PATH)"
	@echo "📡 通信方式: stdio (标准输入输出)"
	@echo "🔌 适用场景: Agent 插件、CLI 工具"
	@echo ""
	@$(MCP_BUILD_DIR)/$(MCP_BINARY_NAME) -config $(MCP_CONFIG_PATH)

# 开发模式运行
mcp-dev:
	@echo "🔧 开发模式启动 CoreX MCP 服务 (stdio 模式)..."
	@echo "📋 配置文件: $(MCP_CONFIG_PATH)"
	@echo "📡 通信方式: stdio (标准输入输出)"
	@echo "🔄 文件变更将自动重启服务"
	@echo ""
	@go run $(MCP_MAIN_PATH) -config $(MCP_CONFIG_PATH)

# 运行测试
mcp-test:
	@echo "🧪 运行 MCP 服务测试..."
	@go test -v ./mcp/...
	@echo "✅ MCP 测试完成"

# 清理编译文件
mcp-clean:
	@echo "🧹 清理 MCP 编译文件..."
	@rm -rf $(MCP_BUILD_DIR)/$(MCP_BINARY_NAME)
	@echo "✅ MCP 清理完成"

# 安装依赖
mcp-install:
	@echo "📦 安装 MCP 服务依赖..."
	@go mod tidy
	@go mod download
	@echo "✅ MCP 依赖安装完成"

# 检查服务状态
mcp-check:
	@echo "🔍 检查 MCP 服务状态..."
	@echo "检查配置文件:"
	@if [ -f $(MCP_CONFIG_PATH) ]; then \
		echo "✅ 配置文件存在: $(MCP_CONFIG_PATH)"; \
		echo "配置文件内容:"; \
		cat $(MCP_CONFIG_PATH); \
	else \
		echo "❌ 配置文件不存在: $(MCP_CONFIG_PATH)"; \
	fi
	@echo ""
	@echo "检查 MCP 工具规范:"
	@ls -la ./mcp/tool_specs/ 2>/dev/null || echo "工具规范目录不存在"

# 快速启动 (别名)
mcp: mcp-dev

# 生产环境部署
mcp-deploy: mcp-build
	@echo "🚀 部署 MCP 服务到生产环境..."
	@echo "请确保已正确配置生产环境参数"
	@$(MCP_BUILD_DIR)/$(MCP_BINARY_NAME) -config $(MCP_CONFIG_PATH)

# 测试 MCP 工具
mcp-test-tools:
	@echo "🧪 测试 MCP 工具..."
	@echo "测试工具列表:"
	@echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | $(MCP_BUILD_DIR)/$(MCP_BINARY_NAME) -config $(MCP_CONFIG_PATH) || echo "工具列表获取失败"
	@echo ""
	@echo "测试 plan_flow 工具:"
	@echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"plan_flow","arguments":{"flow":{"id":"test","name":"测试流程","steps":[]}}}}' | $(MCP_BUILD_DIR)/$(MCP_BINARY_NAME) -config $(MCP_CONFIG_PATH) || echo "工具调用失败"