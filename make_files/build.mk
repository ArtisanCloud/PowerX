# 构建相关的 Makefile
# 包含所有构建任务和编译相关的命令

# 构建目录
BUILD_DIR ?= bin

# Go 编译参数
GO_BUILD_FLAGS ?= -v -ldflags="-s -w"
GO_TEST_FLAGS ?= -v

# 构建所有组件
.PHONY: build build-all build-agent build-test build-demo
build: build-all

build-all: build-agent build-test build-demo
	@echo "✅ 所有组件构建完成"

# 构建 Agent 服务
build-agent:
	@echo "🔨 构建 Agent 服务..."
	@mkdir -p $(BUILD_DIR)
	go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/agent ./backend/cmd/agent/
	@echo "✅ Agent 服务构建完成"

# 构建测试工具
build-test:
	@echo "🔨 构建测试工具..."
	@mkdir -p $(BUILD_DIR)
	go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/test-agent-api ./backend/cmd/test_agent_api/
	@echo "✅ 测试工具构建完成"

# 构建演示程序
build-demo:
	@echo "🔨 构建演示程序..."
	@mkdir -p $(BUILD_DIR)
	go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/demo ./backend/cmd/demo/
	@echo "✅ 演示程序构建完成"

# 构建 Eino Agent 验证工具
build-verify:
	@echo "🔨 构建验证工具..."
	@mkdir -p $(BUILD_DIR)
	go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/verify-setup ./temp/agent/verify_setup.go
	@echo "✅ 验证工具构建完成"

# 交叉编译
.PHONY: build-linux build-windows build-darwin build-cross
build-linux:
	@echo "🐧 构建 Linux 版本..."
	@mkdir -p $(BUILD_DIR)/linux
	GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/linux/agent ./backend/cmd/agent/
	GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/linux/demo ./backend/cmd/demo/
	@echo "✅ Linux 版本构建完成"

build-windows:
	@echo "🪟 构建 Windows 版本..."
	@mkdir -p $(BUILD_DIR)/windows
	GOOS=windows GOARCH=amd64 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/windows/agent.exe ./backend/cmd/agent/
	GOOS=windows GOARCH=amd64 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/windows/demo.exe ./backend/cmd/demo/
	@echo "✅ Windows 版本构建完成"

build-darwin:
	@echo "🍎 构建 macOS 版本..."
	@mkdir -p $(BUILD_DIR)/darwin
	GOOS=darwin GOARCH=amd64 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/darwin/agent ./backend/cmd/agent/
	GOOS=darwin GOARCH=amd64 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/darwin/demo ./backend/cmd/demo/
	@echo "✅ macOS 版本构建完成"

build-cross: build-linux build-windows build-darwin
	@echo "🌍 所有平台版本构建完成"

# 构建优化版本
.PHONY: build-release build-debug
build-release:
	@echo "🚀 构建发布版本..."
	@mkdir -p $(BUILD_DIR)/release
	go build -ldflags="-s -w -X main.version=$(shell git describe --tags --always)" -o $(BUILD_DIR)/release/agent ./backend/cmd/agent/
	go build -ldflags="-s -w -X main.version=$(shell git describe --tags --always)" -o $(BUILD_DIR)/release/demo ./backend/cmd/demo/
	@echo "✅ 发布版本构建完成"

build-debug:
	@echo "🐛 构建调试版本..."
	@mkdir -p $(BUILD_DIR)/debug
	go build -gcflags="all=-N -l" -o $(BUILD_DIR)/debug/agent ./backend/cmd/agent/
	go build -gcflags="all=-N -l" -o $(BUILD_DIR)/debug/demo ./backend/cmd/demo/
	@echo "✅ 调试版本构建完成"

# 依赖管理
.PHONY: deps deps-update deps-tidy deps-vendor
deps:
	@echo "📦 安装依赖..."
	go mod download
	@echo "✅ 依赖安装完成"

deps-update:
	@echo "🔄 更新依赖..."
	go get -u ./...
	go mod tidy
	@echo "✅ 依赖更新完成"

deps-tidy:
	@echo "🧹 整理依赖..."
	go mod tidy
	@echo "✅ 依赖整理完成"

deps-vendor:
	@echo "📁 创建 vendor 目录..."
	go mod vendor
	@echo "✅ vendor 目录创建完成"

# 检查构建环境
.PHONY: check-env
check-env:
	@echo "🔍 检查构建环境..."
	@echo "Go 版本: $(shell go version)"
	@echo "GOOS: $(shell go env GOOS)"
	@echo "GOARCH: $(shell go env GOARCH)"
	@echo "GOPATH: $(shell go env GOPATH)"
	@echo "GOROOT: $(shell go env GOROOT)"
	@echo "构建目录: $(BUILD_DIR)"
	@echo "构建参数: $(GO_BUILD_FLAGS)"
	@echo "✅ 环境检查完成"