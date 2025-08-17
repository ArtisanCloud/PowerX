# 清理相关的 Makefile
# 包含所有清理任务和维护相关的命令

# 清理配置
BUILD_DIR ?= bin
TEMP_DIR ?= temp
LOGS_DIR ?= logs
REPORTS_DIR ?= reports
DOCS_DIR ?= docs

# 基础清理
.PHONY: clean clean-all clean-build clean-temp clean-logs clean-reports clean-docs

# 默认清理（清理构建文件）
clean: clean-build
	@echo "✅ 基础清理完成"

# 深度清理（清理所有生成的文件）
clean-all: clean-build clean-temp clean-logs clean-reports clean-docs clean-cache
	@echo "🧹 深度清理完成"

# 清理构建文件
clean-build:
	@echo "🔨 清理构建文件..."
	@rm -rf $(BUILD_DIR)/
	@rm -f *.exe
	@rm -f *.test
	@rm -f coverage.out
	@go clean -cache
	@go clean -testcache
	@go clean -modcache
	@echo "✅ 构建文件清理完成"

# 清理临时文件
clean-temp:
	@echo "🗑️  清理临时文件..."
	@find . -name "*.tmp" -type f -delete
	@find . -name "*.temp" -type f -delete
	@find . -name ".DS_Store" -type f -delete
	@find . -name "Thumbs.db" -type f -delete
	@rm -rf .air_tmp/
	@echo "✅ 临时文件清理完成"

# 清理日志文件
clean-logs:
	@echo "📋 清理日志文件..."
	@rm -rf $(LOGS_DIR)/
	@rm -f *.log
	@echo "✅ 日志文件清理完成"

# 清理测试报告
clean-reports:
	@echo "📊 清理测试报告..."
	@rm -rf $(REPORTS_DIR)/
	@rm -f test-results.json
	@rm -f coverage.html
	@echo "✅ 测试报告清理完成"

# 清理文档
clean-docs:
	@echo "📚 清理生成的文档..."
	@rm -rf $(DOCS_DIR)/api/
	@rm -rf $(DOCS_DIR)/code/
	@echo "✅ 文档清理完成"

# 清理缓存
clean-cache:
	@echo "💾 清理缓存..."
	@go clean -cache
	@go clean -testcache
	@go clean -fuzzcache
	@echo "✅ 缓存清理完成"

# 清理依赖缓存
clean-deps:
	@echo "📦 清理依赖缓存..."
	@go clean -modcache
	@rm -rf vendor/
	@echo "✅ 依赖缓存清理完成"

# 清理 Git 相关
.PHONY: clean-git clean-branches
clean-git:
	@echo "🌿 清理 Git 临时文件..."
	@git clean -fd
	@echo "✅ Git 临时文件清理完成"

clean-branches:
	@echo "🌿 清理已合并的分支..."
	@git branch --merged | grep -v "\*\|main\|master\|develop" | xargs -n 1 git branch -d
	@echo "✅ 已合并分支清理完成"

# 清理 IDE 文件
.PHONY: clean-ide clean-vscode clean-idea
clean-ide: clean-vscode clean-idea
	@echo "✅ IDE 文件清理完成"

clean-vscode:
	@echo "💻 清理 VSCode 文件..."
	@rm -rf .vscode/settings.json.bak
	@find . -name "*.code-workspace" -type f -delete
	@echo "✅ VSCode 文件清理完成"

clean-idea:
	@echo "💡 清理 IntelliJ IDEA 文件..."
	@rm -rf .idea/workspace.xml.bak
	@rm -rf .idea/tasks.xml
	@echo "✅ IntelliJ IDEA 文件清理完成"

# 清理系统文件
.PHONY: clean-system
clean-system:
	@echo "🖥️  清理系统文件..."
	@find . -name ".DS_Store" -type f -delete
	@find . -name "Thumbs.db" -type f -delete
	@find . -name "desktop.ini" -type f -delete
	@find . -name "*.swp" -type f -delete
	@find . -name "*.swo" -type f -delete
	@find . -name "*~" -type f -delete
	@echo "✅ 系统文件清理完成"

# 清理 Docker 相关
.PHONY: clean-docker
clean-docker:
	@echo "🐳 清理 Docker 文件..."
	@if command -v docker > /dev/null; then \
		docker system prune -f; \
		docker volume prune -f; \
		echo "✅ Docker 清理完成"; \
	else \
		echo "⚠️  Docker 未安装，跳过清理"; \
	fi

# 清理 Node.js 相关（如果项目中有前端部分）
.PHONY: clean-node
clean-node:
	@echo "📦 清理 Node.js 文件..."
	@rm -rf node_modules/
	@rm -f package-lock.json
	@rm -f yarn.lock
	@echo "✅ Node.js 文件清理完成"

# 重置项目到初始状态
.PHONY: reset reset-hard
reset:
	@echo "🔄 重置项目..."
	@make clean-all
	@go mod tidy
	@echo "✅ 项目重置完成"

reset-hard:
	@echo "⚠️  硬重置项目（将删除所有未跟踪的文件）..."
	@read -p "确定要继续吗？(y/N): " confirm && [ "$$confirm" = "y" ] || exit 1
	@git clean -fdx
	@git reset --hard HEAD
	@make clean-all
	@go mod tidy
	@echo "✅ 项目硬重置完成"

# 维护相关
.PHONY: maintenance check-disk check-permissions fix-permissions
maintenance: clean-all check-disk check-permissions
	@echo "🔧 项目维护完成"

# 检查磁盘空间
check-disk:
	@echo "💾 检查磁盘空间..."
	@df -h . | head -2
	@echo "当前目录大小:"
	@du -sh .
	@echo "✅ 磁盘空间检查完成"

# 检查文件权限
check-permissions:
	@echo "🔐 检查文件权限..."
	@find . -name "*.sh" -type f ! -perm -u+x -exec echo "需要执行权限: {}" \;
	@echo "✅ 文件权限检查完成"

# 修复文件权限
fix-permissions:
	@echo "🔧 修复文件权限..."
	@find . -name "*.sh" -type f -exec chmod +x {} \;
	@find . -name "*.mk" -type f -exec chmod 644 {} \;
	@echo "✅ 文件权限修复完成"

# 清理统计
.PHONY: clean-stats
clean-stats:
	@echo "📊 清理前统计:"
	@echo "构建文件: $(shell find $(BUILD_DIR) -type f 2>/dev/null | wc -l || echo 0) 个"
	@echo "临时文件: $(shell find . -name "*.tmp" -o -name "*.temp" | wc -l) 个"
	@echo "日志文件: $(shell find $(LOGS_DIR) -type f 2>/dev/null | wc -l || echo 0) 个"
	@echo "测试文件: $(shell find . -name "*.test" | wc -l) 个"
	@echo "缓存大小: $(shell du -sh ~/.cache/go-build 2>/dev/null | cut -f1 || echo '未知')"

# 清理验证
.PHONY: verify-clean
verify-clean:
	@echo "✅ 验证清理结果:"
	@echo "构建目录: $(shell [ -d $(BUILD_DIR) ] && echo '存在' || echo '已清理')"
	@echo "临时文件: $(shell find . -name "*.tmp" -o -name "*.temp" | wc -l) 个"
	@echo "日志目录: $(shell [ -d $(LOGS_DIR) ] && echo '存在' || echo '已清理')"
	@echo "测试文件: $(shell find . -name "*.test" | wc -l) 个"
	@if [ $(shell find . -name "*.tmp" -o -name "*.temp" | wc -l) -eq 0 ]; then \
		echo "🎉 清理验证通过"; \
	else \
		echo "⚠️  仍有临时文件存在"; \
	fi

# 帮助信息
.PHONY: clean-help
clean-help:
	@echo "🧹 清理命令帮助:"
	@echo ""
	@echo "基础清理:"
	@echo "  make clean           - 清理构建文件"
	@echo "  make clean-build     - 清理构建文件和缓存"
	@echo "  make clean-temp      - 清理临时文件"
	@echo "  make clean-logs      - 清理日志文件"
	@echo ""
	@echo "深度清理:"
	@echo "  make clean-all       - 清理所有生成的文件"
	@echo "  make clean-cache     - 清理所有缓存"
	@echo "  make clean-deps      - 清理依赖缓存"
	@echo ""
	@echo "特殊清理:"
	@echo "  make clean-git       - 清理 Git 临时文件"
	@echo "  make clean-ide       - 清理 IDE 文件"
	@echo "  make clean-docker    - 清理 Docker 文件"
	@echo ""
	@echo "项目重置:"
	@echo "  make reset           - 重置项目到干净状态"
	@echo "  make reset-hard      - 硬重置（删除所有未跟踪文件）"
	@echo ""
	@echo "维护工具:"
	@echo "  make maintenance     - 运行项目维护"
	@echo "  make clean-stats     - 显示清理统计"
	@echo "  make verify-clean    - 验证清理结果"