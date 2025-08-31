# ==== OpenAPI & Permission Sync ====
# 生成 Swagger 文档（或最小 OpenAPI）并同步权限目录到数据库

# ---- 可调参数（命令行可覆盖）----
APP_MAIN      ?= cmd/app/main.go
SWAG_VERSION  ?= v1.16.3

# Swagger 输出目录（注意：这里可能还放了其它文档，所以清理时只删生成的3个文件）
DOCS_DIR      ?= ./docs
SWAG_JSON     ?= $(DOCS_DIR)/swagger.json
SWAG_YAML     ?= $(DOCS_DIR)/swagger.yaml
SWAG_GO       ?= $(DOCS_DIR)/docs.go

# 最小 OpenAPI（运行中的服务需已挂 /openapi.min.json）
PORT          ?= 8077
MIN_OPENAPI   ?= http://localhost:$(PORT)/openapi.min.json

# 权限同步参数
SOURCE        ?= core
INTRODUCED    ?= v1.0.0

GO            ?= go
GOBIN         ?= $(shell $(GO) env GOPATH)/bin
SWAG          ?= $(GOBIN)/swag

# ---- 工具安装 & 依赖对齐 ----
.PHONY: tools.swag deps.swag
tools.swag:
	@echo ">> install swag CLI $(SWAG_VERSION)"
	@$(GO) install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION)

# 可选：将项目依赖对齐到与 CLI 一致的版本，避免 LeftDelim/RightDelim 不兼容
deps.swag:
	@echo ">> align project deps to swag $(SWAG_VERSION)"
	@$(GO) get github.com/swaggo/swag@$(SWAG_VERSION)
	@$(GO) get github.com/swaggo/gin-swagger@v1.6.0
	@$(GO) get github.com/swaggo/files@v1.3.3
	@$(GO) mod tidy

# ---- 生成标准 Swagger 文档（基于注解）----
.PHONY: swagger.gen swagger.clean swagger.verify
swagger.gen: tools.swag
	@echo ">> generate swagger docs to $(DOCS_DIR)"
	@mkdir -p $(DOCS_DIR)
	@$(SWAG) init -g $(APP_MAIN) -o $(DOCS_DIR)

# 仅清理 swagger 生成的三个文件，避免误删 docs/ 下的其它资料
swagger.clean:
	@echo ">> clean swagger generated files"
	@rm -f $(SWAG_JSON) $(SWAG_YAML) $(SWAG_GO)

# 简单校验
swagger.verify:
	@test -f "$(SWAG_JSON)" || (echo "swagger.json not found at $(SWAG_JSON)"; exit 1)

# ---- 基于标准 swagger.json 的权限同步 ----
.PHONY: permgen.print permgen.apply
permgen.print: swagger.verify
	@echo ">> dry-run from $(SWAG_JSON)"
	@$(GO) run ./cmd/perm_gen -openapi $(SWAG_JSON) -source $(SOURCE) -introduced $(INTRODUCED)

permgen.apply: swagger.verify
	@echo ">> APPLY from $(SWAG_JSON)"
	@$(GO) run ./cmd/perm_gen -openapi $(SWAG_JSON) -source $(SOURCE) -introduced $(INTRODUCED) -apply

# ---- 基于“最小 OpenAPI”（/openapi.min.json）的权限同步 ----
.PHONY: permgen.min.print permgen.min.apply
permgen.min.print:
	@echo ">> dry-run from $(MIN_OPENAPI)"
	@$(GO) run ./cmd/perm_gen -openapi $(MIN_OPENAPI) -source $(SOURCE) -introduced $(INTRODUCED)

permgen.min.apply:
	@echo ">> APPLY from $(MIN_OPENAPI)"
	@$(GO) run ./cmd/perm_gen -openapi $(MIN_OPENAPI) -source $(SOURCE) -introduced $(INTRODUCED) -apply

.PHONY: perm.seed
perm.seed: permgen.apply
	@echo "✅ 权限已同步（基于 docs/swagger.json）"

# 先生成注解版 swagger，再同步（只有在你走注解流时用）
.PHONY: perm.seed.from-swag
perm.seed.from-swag: swagger.gen permgen.apply
	@echo "✅ 权限已同步（基于 swagger.gen）"