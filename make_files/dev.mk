# 开发相关的 Makefile
# 包含所有开发任务和代码质量相关的命令

# 开发配置
DEV_PORT ?= 8077
DEV_HOST ?= localhost
LOG_LEVEL ?= debug

# 启动开发服务器
.PHONY: dev dev-agent dev-demo dev-watch
dev: dev-demo

# 启动 Agent 服务
dev-agent:
	@echo "🚀 启动 Agent 开发服务器..."
	@echo "服务地址: http://$(DEV_HOST):$(DEV_PORT)"
	@echo "按 Ctrl+C 停止服务器"
	@LOG_LEVEL=$(LOG_LEVEL) go run cmd/agent/main.go

# 启动演示服务器
dev-demo:
	@echo "🚀 启动演示开发服务器..."
	@echo "服务地址: http://$(DEV_HOST):$(DEV_PORT)"
	@echo "API 文档: http://$(DEV_HOST):$(DEV_PORT)/api/v1/docs"
	@echo "测试页面: temp/agent/test.html"
	@echo "按 Ctrl+C 停止服务器"
	@LOG_LEVEL=$(LOG_LEVEL) go run cmd/demo/main.go

# 监控文件变化并自动重启（需要安装 air）
dev-watch:
	@echo "👀 启动文件监控模式..."
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "❌ 未找到 air 工具，请先安装:"; \
		echo "go install github.com/cosmtrek/air@latest"; \
		echo "或使用: make dev"; \
	fi

# 代码质量检查
.PHONY: lint format vet check-all
lint: vet format
	@echo "✅ 代码检查完成"

# 代码格式化
format:
	@echo "🎨 格式化代码..."
	@go fmt ./...
	@echo "✅ 代码格式化完成"

# 代码静态分析
vet:
	@echo "🔍 静态代码分析..."
	@go vet ./...
	@echo "✅ 静态分析完成"

# 全面代码检查
check-all: format vet
	@echo "🔍 运行全面代码检查..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "⚠️  未找到 golangci-lint，跳过高级检查"; \
		echo "安装命令: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi
	@echo "✅ 全面检查完成"

# 代码生成
.PHONY: generate mock proto
generate:
	@echo "🔧 生成代码..."
	@go generate ./...
	@echo "✅ 代码生成完成"

# 生成 Mock 文件
mock:
	@echo "🎭 生成 Mock 文件..."
	@if command -v mockgen > /dev/null; then \
		mockgen -source=pkg/agent/contract/agent.go -destination=pkg/agent/contract/mocks/agent_mock.go; \
		echo "✅ Mock 文件生成完成"; \
	else \
		echo "❌ 未找到 mockgen 工具，请先安装:"; \
		echo "go install github.com/golang/mock/mockgen@latest"; \
	fi

# 生成 Protocol Buffers（如果需要）
proto:
	@echo "📡 生成 Protocol Buffers..."
	@if command -v protoc > /dev/null; then \
		find . -name "*.proto" -exec protoc --go_out=. --go-grpc_out=. {} \; ; \
		echo "✅ Protocol Buffers 生成完成"; \
	else \
		echo "⚠️  未找到 protoc，跳过 proto 生成"; \
	fi

# 文档生成
.PHONY: docs docs-api docs-code
docs: docs-api docs-code

# 生成 API 文档
docs-api:
	@echo "📚 生成 API 文档..."
	@if command -v swag > /dev/null; then \
		swag init -g cmd/demo/main.go -o docs/api; \
		echo "✅ API 文档生成完成: docs/api/"; \
	else \
		echo "❌ 未找到 swag 工具，请先安装:"; \
		echo "go install github.com/swaggo/swag/cmd/swag@latest"; \
	fi

# 生成代码文档
docs-code:
	@echo "📖 生成代码文档..."
	@mkdir -p docs/code
	@go doc -all ./pkg/agent/drivers/eino > docs/code/eino-agent.md
	@go doc -all ./api/http/agent > docs/code/api.md
	@echo "✅ 代码文档生成完成: docs/code/"

# 开发工具安装
.PHONY: install-tools install-air install-lint install-swag
install-tools: install-air install-lint install-swag
	@echo "✅ 所有开发工具安装完成"

install-air:
	@echo "💨 安装 Air 热重载工具..."
	@go install github.com/cosmtrek/air@latest
	@echo "✅ Air 安装完成"

install-lint:
	@echo "🔍 安装 GolangCI-Lint..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "✅ GolangCI-Lint 安装完成"

install-swag:
	@echo "📚 安装 Swag API 文档工具..."
	@go install github.com/swaggo/swag/cmd/swag@latest
	@echo "✅ Swag 安装完成"

# 开发环境初始化
.PHONY: dev-init dev-setup
dev-init: deps install-tools
	@echo "🎯 初始化开发环境..."
	@mkdir -p docs/api docs/code reports logs
	@echo "✅ 开发环境初始化完成"

dev-setup: dev-init
	@echo "⚙️  设置开发环境..."
	@if [ ! -f .air.toml ]; then \
		echo "创建 Air 配置文件..."; \
		air init; \
	fi
	@if [ ! -f .golangci.yml ]; then \
		echo "创建 GolangCI-Lint 配置文件..."; \
		echo "# GolangCI-Lint 配置" > .golangci.yml; \
		echo "run:" >> .golangci.yml; \
		echo "  timeout: 5m" >> .golangci.yml; \
	fi
	@echo "✅ 开发环境设置完成"

# 调试相关
.PHONY: debug debug-agent debug-demo
debug:
	@echo "🐛 启动调试模式..."
	@echo "使用 dlv 调试器，请确保已安装: go install github.com/go-delve/delve/cmd/dlv@latest"

debug-agent:
	@echo "🐛 调试 Agent 服务..."
	@if command -v dlv > /dev/null; then \
		dlv debug cmd/agent/main.go; \
	else \
		echo "❌ 未找到 dlv 调试器，请先安装:"; \
		echo "go install github.com/go-delve/delve/cmd/dlv@latest"; \
	fi

debug-demo:
	@echo "🐛 调试演示服务..."
	@if command -v dlv > /dev/null; then \
		dlv debug cmd/demo/main.go; \
	else \
		echo "❌ 未找到 dlv 调试器，请先安装:"; \
		echo "go install github.com/go-delve/delve/cmd/dlv@latest"; \
	fi

# 性能分析
.PHONY: profile profile-cpu profile-mem
profile:
	@echo "📊 启动性能分析..."
	@echo "访问 http://$(DEV_HOST):$(DEV_PORT)/debug/pprof/ 查看性能数据"

profile-cpu:
	@echo "🔥 CPU 性能分析..."
	@go tool pprof http://$(DEV_HOST):$(DEV_PORT)/debug/pprof/profile

profile-mem:
	@echo "💾 内存性能分析..."
	@go tool pprof http://$(DEV_HOST):$(DEV_PORT)/debug/pprof/heap

# 日志查看
.PHONY: logs logs-tail logs-error
logs:
	@echo "📋 查看应用日志..."
	@if [ -f logs/app.log ]; then \
		cat logs/app.log; \
	else \
		echo "❌ 日志文件不存在"; \
	fi

logs-tail:
	@echo "👀 实时查看日志..."
	@if [ -f logs/app.log ]; then \
		tail -f logs/app.log; \
	else \
		echo "❌ 日志文件不存在"; \
	fi

logs-error:
	@echo "❌ 查看错误日志..."
	@if [ -f logs/error.log ]; then \
		cat logs/error.log; \
	else \
		echo "❌ 错误日志文件不存在"; \
	fi

# 开发环境状态检查
.PHONY: dev-status
dev-status:
	@echo "📊 开发环境状态:"
	@echo "Go 版本: $(shell go version)"
	@echo "开发端口: $(DEV_PORT)"
	@echo "开发主机: $(DEV_HOST)"
	@echo "日志级别: $(LOG_LEVEL)"
	@echo ""
	@echo "已安装的开发工具:"
	@command -v air > /dev/null && echo "✅ Air (热重载)" || echo "❌ Air (未安装)"
	@command -v golangci-lint > /dev/null && echo "✅ GolangCI-Lint (代码检查)" || echo "❌ GolangCI-Lint (未安装)"
	@command -v swag > /dev/null && echo "✅ Swag (API文档)" || echo "❌ Swag (未安装)"
	@command -v dlv > /dev/null && echo "✅ Delve (调试器)" || echo "❌ Delve (未安装)"
	@command -v mockgen > /dev/null && echo "✅ MockGen (Mock生成)" || echo "❌ MockGen (未安装)"