# Protobuf & Contracts related targets

BUF_CMD ?= $(shell command -v buf)
BUF_WORKDIR ?= api/grpc/contracts

.PHONY: proto-gen proto-event-fabric proto-lint proto-clean contracts-test ci-proto

proto-gen:
	@echo "🛠️  生成 Protobuf 代码..."
	@if [ -z "$(BUF_CMD)" ]; then \
		echo "❌ 未找到 buf CLI，请先安装: https://buf.build/docs/installation"; \
		exit 1; \
	fi
	@cd $(BUF_WORKDIR) && buf generate --template buf.gen.yaml
	@echo "✅ Protobuf 代码生成完成"

proto-event-fabric:
	@echo "🛠️  生成 Event Fabric Protobuf 代码..."
	@if [ -z "$(BUF_CMD)" ]; then \
		echo "❌ 未找到 buf CLI，请先安装: https://buf.build/docs/installation"; \
		exit 1; \
	fi
	@cd $(BUF_WORKDIR) && buf generate --template buf.gen.yaml --path corex/event_fabric/v1
	@echo "✅ Event Fabric Protobuf 代码生成完成"

proto-lint:
	@echo "🔍 运行 Protobuf Lint..."
	@if [ -z "$(BUF_CMD)" ]; then \
		echo "❌ 未找到 buf CLI，请先安装: https://buf.build/docs/installation"; \
		exit 1; \
	fi
	@cd $(BUF_WORKDIR) && buf lint
	@echo "✅ Lint 检查通过"

proto-clean:
	@echo "🧹 清理生成的 Protobuf 代码..."
	@rm -rf api/grpc/gen/go/powerx/corex
	@rm -rf api/grpc/gen/go/powerx/capability
	@rm -rf api/grpc/gen/go/corex/event_fabric
	@find api/grpc/gen -type f -name "*.pb.go" -empty -delete >/dev/null 2>&1 || true
	@echo "✅ 清理完成"

contracts-test:
	@echo "🧪 运行契约兼容性测试..."
	@if [ -z "$(BUF_CMD)" ]; then \
		echo "❌ 未找到 buf CLI，请先安装: https://buf.build/docs/installation"; \
		exit 1; \
	fi
	@cd $(BUF_WORKDIR) && buf lint
	@if git rev-parse --verify main >/dev/null 2>&1; then \
		cd $(BUF_WORKDIR) && buf breaking --against '.git#branch=main'; \
	else \
		echo "⚠️  未找到 main 分支，跳过 breaking 检查"; \
	fi
	@echo "✅ 契约测试完成"

ci-proto: proto-lint contracts-test
	@echo "✅ CI Protobuf 钩子执行完成"
