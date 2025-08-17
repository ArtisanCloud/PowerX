# Makefile 全局配置文件
# 定义所有 Makefile 共享的变量和配置

# 项目信息
PROJECT_NAME := eino-agent-api
PROJECT_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.1.0")
PROJECT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# 构建配置
BUILD_DIR ?= bin
TEMP_DIR ?= temp
LOGS_DIR ?= logs
REPORTS_DIR ?= reports
DOCS_DIR ?= docs

# Go 编译配置
GO_VERSION := $(shell go version | cut -d' ' -f3)
GO_BUILD_FLAGS ?= -v -ldflags="-s -w -X main.version=$(PROJECT_VERSION) -X main.commit=$(PROJECT_COMMIT) -X main.buildTime=$(BUILD_TIME)"
GO_TEST_FLAGS ?= -v -race
GO_LINT_FLAGS ?= --timeout=5m

# 开发服务器配置
DEV_PORT ?= 8080
DEV_HOST ?= localhost
LOG_LEVEL ?= debug

# API 测试配置
API_BASE_URL ?= http://$(DEV_HOST):$(DEV_PORT)/api/v1
TEST_TIMEOUT ?= 30s
TEST_VERBOSE ?= -v

# Docker 配置（如果需要）
DOCKER_IMAGE ?= $(PROJECT_NAME)
DOCKER_TAG ?= $(PROJECT_VERSION)
DOCKER_REGISTRY ?= 

# 颜色定义（用于终端输出）
COLOR_RESET := \033[0m
COLOR_RED := \033[31m
COLOR_GREEN := \033[32m
COLOR_YELLOW := \033[33m
COLOR_BLUE := \033[34m
COLOR_PURPLE := \033[35m
COLOR_CYAN := \033[36m
COLOR_WHITE := \033[37m

# 图标定义
ICON_SUCCESS := ✅
ICON_ERROR := ❌
ICON_WARNING := ⚠️
ICON_INFO := ℹ️
ICON_BUILD := 🔨
ICON_TEST := 🧪
ICON_CLEAN := 🧹
ICON_DEV := 🚀
ICON_DOC := 📚

# 工具检查函数
define check_tool
	@if ! command -v $(1) > /dev/null 2>&1; then \
		echo "$(COLOR_RED)$(ICON_ERROR) 未找到工具: $(1)$(COLOR_RESET)"; \
		echo "$(COLOR_YELLOW)安装命令: $(2)$(COLOR_RESET)"; \
		exit 1; \
	fi
endef

# 成功消息函数
define success_msg
	@echo "$(COLOR_GREEN)$(ICON_SUCCESS) $(1)$(COLOR_RESET)"
endef

# 错误消息函数
define error_msg
	@echo "$(COLOR_RED)$(ICON_ERROR) $(1)$(COLOR_RESET)"
endef

# 警告消息函数
define warning_msg
	@echo "$(COLOR_YELLOW)$(ICON_WARNING) $(1)$(COLOR_RESET)"
endef

# 信息消息函数
define info_msg
	@echo "$(COLOR_BLUE)$(ICON_INFO) $(1)$(COLOR_RESET)"
endef

# 创建目录函数
define create_dir
	@mkdir -p $(1)
	@echo "$(COLOR_CYAN)创建目录: $(1)$(COLOR_RESET)"
endef

# 检查服务器状态函数
define check_server
	@echo "$(COLOR_BLUE)检查服务器状态...$(COLOR_RESET)"
	@if curl -s --connect-timeout 5 $(API_BASE_URL)/agents/health > /dev/null 2>&1; then \
		echo "$(COLOR_GREEN)$(ICON_SUCCESS) 服务器运行正常$(COLOR_RESET)"; \
	else \
		echo "$(COLOR_RED)$(ICON_ERROR) 服务器未运行，请先启动: make dev$(COLOR_RESET)"; \
		exit 1; \
	fi
endef

# 环境变量导出
export PROJECT_NAME
export PROJECT_VERSION
export PROJECT_COMMIT
export BUILD_TIME
export BUILD_DIR
export TEMP_DIR
export LOGS_DIR
export REPORTS_DIR
export DOCS_DIR
export GO_BUILD_FLAGS
export GO_TEST_FLAGS
export DEV_PORT
export DEV_HOST
export LOG_LEVEL
export API_BASE_URL
export TEST_TIMEOUT
export TEST_VERBOSE

# 显示配置信息
.PHONY: show-config
show-config:
	@echo "$(COLOR_CYAN)📋 项目配置信息:$(COLOR_RESET)"
	@echo "项目名称: $(PROJECT_NAME)"
	@echo "项目版本: $(PROJECT_VERSION)"
	@echo "提交哈希: $(PROJECT_COMMIT)"
	@echo "构建时间: $(BUILD_TIME)"
	@echo "Go 版本: $(GO_VERSION)"
	@echo ""
	@echo "$(COLOR_CYAN)📁 目录配置:$(COLOR_RESET)"
	@echo "构建目录: $(BUILD_DIR)"
	@echo "临时目录: $(TEMP_DIR)"
	@echo "日志目录: $(LOGS_DIR)"
	@echo "报告目录: $(REPORTS_DIR)"
	@echo "文档目录: $(DOCS_DIR)"
	@echo ""
	@echo "$(COLOR_CYAN)🚀 开发配置:$(COLOR_RESET)"
	@echo "开发主机: $(DEV_HOST)"
	@echo "开发端口: $(DEV_PORT)"
	@echo "日志级别: $(LOG_LEVEL)"
	@echo "API 基础URL: $(API_BASE_URL)"
	@echo ""
	@echo "$(COLOR_CYAN)🧪 测试配置:$(COLOR_RESET)"
	@echo "测试超时: $(TEST_TIMEOUT)"
	@echo "详细输出: $(TEST_VERBOSE)"