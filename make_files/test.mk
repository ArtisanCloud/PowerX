# 测试相关的 Makefile
# 包含所有测试任务和验证相关的命令

# 测试配置
API_BASE_URL ?= http://localhost:8077/api/v1
TEST_TIMEOUT ?= 30s
TEST_VERBOSE ?= -v
API_AUTH_TOKEN ?=
API_TENANT_UUID ?=

ifdef API_AUTH_TOKEN
CURL_AUTH_FLAGS := -H "Authorization: Bearer $(API_AUTH_TOKEN)"
endif
ifdef API_TENANT_UUID
CURL_TENANT_FLAGS := -H "tenant-uuid: $(API_TENANT_UUID)"
endif
CURL_BASE := curl -s -S $(CURL_AUTH_FLAGS) $(CURL_TENANT_FLAGS)

# 基础测试命令
.PHONY: test-health test-chat test-plan test-execute test-config test-batch test-all test-quick
.PHONY: log-unification-check

log-unification-check:
	@bash backend/scripts/ci/log-unification-check.sh

# 测试健康检查
test-health:
	@echo "🏥 测试健康检查..."
	@tmp=$$(mktemp); \
	code=$$($(CURL_BASE) $(API_BASE_URL)/health -w "%{http_code}" -o $$tmp); \
	if [ "$$code" -ge 200 ] && [ "$$code" -lt 300 ]; then \
		rm -f $$tmp; \
		echo "✅ 健康检查通过"; \
	else \
		echo "❌ 健康检查失败 (HTTP $$code)"; \
		cat $$tmp; \
		rm -f $$tmp; \
		exit 1; \
	fi

.PHONY: ensure-api-auth
ensure-api-auth:
	@if [ -z "$(API_AUTH_TOKEN)" ]; then \
		echo "❌ 缺少 API_AUTH_TOKEN 环境变量，无法访问受保护 API。"; \
		echo "   请先调用 /api/v1/admin/user/auth/login 获取 token，并执行: export API_AUTH_TOKEN=<token>"; \
		exit 1; \
	fi

# 测试基本聊天
test-chat: ensure-api-auth
	@echo "💬 测试基本聊天..."
	@tmp=$$(mktemp); \
	code=$$($(CURL_BASE) -X POST $(API_BASE_URL)/agents/invoke \
		-H "Content-Type: application/json" \
		-d '{"message":"你好","config":{"model_name":"demo-chat","temperature":0.5}}' \
		-w "%{http_code}" -o $$tmp); \
	if [ "$$code" -ge 200 ] && [ "$$code" -lt 300 ]; then \
		if jq . > /dev/null < $$tmp; then \
			echo "✅ 聊天测试通过"; \
		else \
			echo "❌ 聊天测试失败（响应不是 JSON）"; \
			cat $$tmp; \
			rm -f $$tmp; \
			exit 1; \
		fi; \
	else \
		echo "❌ 聊天测试失败 (HTTP $$code)"; \
		cat $$tmp; \
		rm -f $$tmp; \
		exit 1; \
	fi; \
	rm -f $$tmp

# 测试计划生成
test-plan: ensure-api-auth
	@echo "📋 测试计划生成..."
	@tmp=$$(mktemp); \
	code=$$($(CURL_BASE) -X POST $(API_BASE_URL)/agents/intent/plan \
		-H "Content-Type: application/json" \
		-d '{"message":"制定学习计划","context":{"subject":"AI"},"config":{"model_name":"demo-chat"}}' \
		-w "%{http_code}" -o $$tmp); \
	if [ "$$code" -ge 200 ] && [ "$$code" -lt 300 ]; then \
		if jq . > /dev/null < $$tmp; then \
			echo "✅ 计划生成测试通过"; \
		else \
			echo "❌ 计划生成测试失败（响应不是 JSON）"; \
			cat $$tmp; \
			rm -f $$tmp; \
			exit 1; \
		fi; \
	else \
		echo "❌ 计划生成测试失败 (HTTP $$code)"; \
		cat $$tmp; \
		rm -f $$tmp; \
		exit 1; \
	fi; \
	rm -f $$tmp

# 测试执行意图
test-execute: ensure-api-auth
	@echo "🚀 测试执行意图..."
	@tmp=$$(mktemp); \
	code=$$($(CURL_BASE) -X POST $(API_BASE_URL)/agents/intent/ \
		-H "Content-Type: application/json" \
		-d '{"message":"分析AI趋势","context":{"focus":"LLM"},"config":{"model_name":"demo-chat"}}' \
		-w "%{http_code}" -o $$tmp); \
	if [ "$$code" -ge 200 ] && [ "$$code" -lt 300 ]; then \
		if jq . > /dev/null < $$tmp; then \
			echo "✅ 执行测试通过"; \
		else \
			echo "❌ 执行测试失败（响应不是 JSON）"; \
			cat $$tmp; \
			rm -f $$tmp; \
			exit 1; \
		fi; \
	else \
		echo "❌ 执行测试失败 (HTTP $$code)"; \
		cat $$tmp; \
		rm -f $$tmp; \
		exit 1; \
	fi; \
	rm -f $$tmp

# 测试配置验证
test-config: ensure-api-auth
	@echo "⚙️ 测试配置验证..."
	@tmp=$$(mktemp); \
	code=$$($(CURL_BASE) "$(API_BASE_URL)/admin/agents/providers?modality=llm" -w "%{http_code}" -o $$tmp); \
	if [ "$$code" -ge 200 ] && [ "$$code" -lt 300 ]; then \
		if jq . > /dev/null < $$tmp; then \
			echo "✅ 配置测试通过"; \
		else \
			echo "❌ 配置测试失败（响应不是 JSON）"; \
			cat $$tmp; \
			rm -f $$tmp; \
			exit 1; \
		fi; \
	else \
		echo "❌ 配置测试失败 (HTTP $$code)"; \
		cat $$tmp; \
		rm -f $$tmp; \
		exit 1; \
	fi; \
	rm -f $$tmp

# 测试批量请求
test-batch:
	@echo "🔄 测试批量请求..."
	@for i in {1..3}; do \
		curl -s $(API_BASE_URL)/health > /dev/null && echo "批量测试 $$i: ✅" || echo "批量测试 $$i: ❌"; \
	done

# 运行所有 API 测试
test-all: test-health test-config test-chat test-plan test-execute test-batch
	@echo "🎉 所有 API 测试完成"

# 快速测试（仅核心功能）
test-quick: test-health test-config
	@echo "⚡ 快速测试完成"

# 使用测试脚本进行测试
.PHONY: test-script test-verify test-browser
test-script:
	@echo "📜 运行测试脚本..."
	@chmod +x temp/agent/test_eino_api.sh
	@./temp/agent/test_eino_api.sh

# 验证基本设置
test-verify:
	@echo "🔍 验证基本设置..."
	@go run temp/agent/verify_setup.go

# 浏览器测试提示
test-browser:
	@echo "🌐 浏览器测试:"
	@echo "请在浏览器中打开: temp/agent/test.html"
	@echo "确保服务器已启动: make dev"

# 单元测试
.PHONY: unit-test unit-test-eino unit-test-api unit-test-all
unit-test: unit-test-all
	@echo ""

unit-test-eino:
	@echo "🧪 运行 Eino Agent 单元测试..."
	@cd backend && bash test-silent.sh

unit-test-api:
	@echo "🧪 运行 API 单元测试..."
	@cd backend && bash test-silent.sh

unit-test-all:
	@echo "🧪 运行所有单元测试..."
	@cd backend && bash test-silent.sh

# 集成测试
.PHONY: integration-test e2e-test
integration-test:
	@echo "🔗 运行集成测试..."
	@echo "启动服务器..."
	@go run cmd/demo/main.go &
	@SERVER_PID=$$!; \
	sleep 3; \
	echo "运行集成测试..."; \
	make test-all; \
	echo "停止服务器..."; \
	kill $$SERVER_PID

e2e-test:
	@echo "🎭 运行端到端测试..."
	@echo "这将启动完整的测试流程..."
	@make integration-test
	@make test-verify
	@echo "✅ 端到端测试完成"

# 性能测试
.PHONY: perf-test load-test stress-test
perf-test:
	@echo "⚡ 运行性能测试..."
	@echo "测试健康检查端点性能..."
	@time curl -s $(API_BASE_URL)/agents/health > /dev/null

load-test:
	@echo "📈 运行负载测试..."
	@echo "并发测试健康检查端点..."
	@for i in {1..10}; do \
		curl -s $(API_BASE_URL)/agents/health > /dev/null & \
	done; \
	wait; \
	echo "✅ 负载测试完成"

stress-test:
	@echo "💪 运行压力测试..."
	@echo "高并发测试..."
	@for i in {1..50}; do \
		curl -s $(API_BASE_URL)/agents/health > /dev/null & \
	done; \
	wait; \
	echo "✅ 压力测试完成"

# 测试报告
.PHONY: test-report test-coverage
test-report:
	@echo "📊 生成测试报告..."
	@mkdir -p reports
	@cd backend && go test -json ./... > reports/test-results.json
	@echo "✅ 测试报告已生成: reports/test-results.json"

test-coverage:
	@echo "📈 生成测试覆盖率报告..."
	@mkdir -p reports
	@cd backend && go test -coverprofile=reports/coverage.out ./...
	@cd backend && go tool cover -html=reports/coverage.out -o reports/coverage.html
	@echo "✅ 覆盖率报告已生成: reports/coverage.html"

# 测试环境检查
.PHONY: test-env-check
test-env-check:
	@echo "🔍 检查测试环境..."
	@echo "API 基础 URL: $(API_BASE_URL)"
	@echo "测试超时: $(TEST_TIMEOUT)"
	@echo "详细输出: $(TEST_VERBOSE)"
	@echo "检查服务器连接..."
	@curl -s --connect-timeout 5 $(API_BASE_URL)/agents/health > /dev/null && \
		echo "✅ 服务器连接正常" || \
		echo "❌ 服务器连接失败，请先启动服务器: make dev"

.PHONY: pre-release-gate coverage-gate perf-smoke
pre-release-gate:
	@bash backend/scripts/ops/pre-release-gate.sh

coverage-gate:
	@bash backend/scripts/ci/coverage-gate.sh

perf-smoke:
	@bash backend/scripts/ci/perf-smoke.sh

# 测试数据清理
.PHONY: test-clean
test-clean:
	@echo "🧹 清理测试数据..."
	@rm -rf reports/
	@rm -f *.test
	@rm -f coverage.out
	@echo "✅ 测试数据清理完成"

.PHONY: regression-pxp
regression-pxp:
	@echo "🧪 运行 Plugin Release & Debug 回归套件..."
	@bash scripts/ci/regression_pxp.sh

.PHONY: ci-all
ci-all:
	@echo "🚦 运行 CI 入口：go test + 回归套件..."
	@cd backend && GOFLAGS="" go test ./...
	@$(MAKE) regression-pxp

.PHONY: test-full-regression
test-full-regression:
	@echo "🧪 运行全量回归（backend contract/integration + web-admin E2E）..."
	@bash scripts/ci/t080_full_regression.sh

.PHONY: test-full-regression-backend
test-full-regression-backend:
	@echo "🧪 运行回归（仅 backend，跳过 E2E）..."
	@SKIP_E2E=1 bash scripts/ci/t080_full_regression.sh

# 兼容旧命令（推荐改用 test-full-regression / test-full-regression-backend）
.PHONY: regression-025 regression-025-no-e2e
regression-025: test-full-regression
regression-025-no-e2e: test-full-regression-backend

.PHONY: check-tenant-id
check-tenant-id:
	@echo "🔎 检查 diff 中是否新增 tenant_id ..."
	@bash scripts/ci/check-no-tenant-id.sh
	@echo "🔎 校验是否仍在手动 uuid.Parse tenant_uuid ..."
	@bash scripts/ci/check-tenant-uuid-canonical.sh

.PHONY: check-tenant-migrations
check-tenant-migrations:
	@echo "🧪 校验 tenant UUID 迁移脚本..."
	@bash scripts/ci/check-tenant-migrations.sh
