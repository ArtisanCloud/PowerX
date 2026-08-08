# ========= 可配置参数（可被环境变量覆盖） =========
PGHOST      ?= localhost
PGPORT      ?= 5432
PGUSER      ?= $(shell whoami)
PGPASSWORD  ?=
PGDATABASE  ?= corex
PGSSLMODE   ?= disable   # local 默认关闭

# 连接到 postgres 系统库做元数据查询
PGURL_BASE  = host=$(PGHOST) port=$(PGPORT) user=$(PGUSER) dbname=postgres sslmode=$(PGSSLMODE)

# ========= 目标 =========

.PHONY: db-check
db-check:
	@echo "检查数据库是否存在: $(PGDATABASE) on $(PGUSER)@$(PGHOST):$(PGPORT)"
	@PGPASSWORD="$(PGPASSWORD)" psql "$(PGURL_BASE)" -tAc "SELECT 1 FROM pg_database WHERE datname='$(PGDATABASE)';" | grep -q 1 && \
	  echo "✅ 数据库已存在" || echo "❌ 数据库不存在"

.PHONY: db-create
db-create:
	@echo "创建数据库（若不存在）: $(PGDATABASE)"
	@PGPASSWORD="$(PGPASSWORD)" psql "$(PGURL_BASE)" -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname='$(PGDATABASE)';" | grep -q 1 || \
	  PGPASSWORD="$(PGPASSWORD)" psql "$(PGURL_BASE)" -c "CREATE DATABASE \"$(PGDATABASE)\";"
	@echo "✅ 完成"

# 你的其他目标不变（已改为跑包路径的写法）：
.PHONY: migrate db-migrate
migrate: db-migrate

db-migrate:
	@echo "执行数据库迁移..."
	@cd backend && $(GO) run ./cmd/database migrate

.PHONY: db-rollback
db-rollback:
	@echo "执行数据库回滚..."
	@cd backend && $(GO) run ./cmd/database rollback

.PHONY: seed db-seed
seed:
	$(MAKE) db-seed
	$(MAKE) capability-seed

db-seed:
	@echo "填充数据库种子数据..."
	@cd backend && $(GO) run ./cmd/database seed

.PHONY: db-refresh
db-refresh:
	@echo "⚠️  将刷新数据库（回滚+迁移+种子），该操作会清空当前数据库数据。"
	@printf "确认继续？[y/N] " ; read ans; case "$$ans" in y|Y|yes|YES) echo "继续执行...";; *) echo "已取消"; exit 1;; esac
	@echo "刷新数据库（回滚+迁移+种子）..."
	@cd backend && $(GO) run ./cmd/database refresh

.PHONY: db-status
db-status:
	@echo "查看数据库迁移状态..."
	@cd backend && $(GO) run ./cmd/database status

.PHONY: iam-migration-report
iam-migration-report:
	@echo "执行 IAM SaaS 语义迁移只读巡检..."
	@cd backend && $(GO) run ./cmd/database iam-report

.PHONY: iam-migration-fix-owner
iam-migration-fix-owner:
	@echo "执行 IAM owner 自动补齐（仅处理有 active admin 的租户）..."
	@cd backend && $(GO) run ./cmd/database iam-fix-owner

.PHONY: iam-role-binding-duplicates-report
iam-role-binding-duplicates-report:
	@echo "巡检 IAM 重复角色绑定..."
	@cd backend && $(GO) run ./cmd/database iam-fix-role-binding-duplicates

.PHONY: iam-role-binding-duplicates-fix
iam-role-binding-duplicates-fix:
	@echo "修复 IAM 重复角色绑定..."
	@cd backend && $(GO) run ./cmd/database iam-fix-role-binding-duplicates -confirm


# 帮助信息
.PHONY: help
help:
	@echo "数据库操作命令:"
	@echo "  make db-migrate    - 执行数据库迁移"
	@echo "  make seed          - 顺序执行数据库种子与 Capability Registry 同步"
	@echo "  make db-rollback   - 回滚数据库迁移"
	@echo "  make db-seed       - 填充 CoreX / Metadata / Workflow Pack 数据库种子数据"
	@echo "  make db-refresh    - 刷新数据库（回滚+迁移+种子）"
	@echo "  make db-status     - 查看数据库迁移状态"
	@echo "  make db-check      - 检查数据库是否存在"
	@echo "  make iam-migration-report     - IAM SaaS 语义迁移只读巡检"
	@echo "  make iam-migration-fix-owner  - 自动补齐缺 owner 且有 active admin 的租户"
	@echo "  make iam-role-binding-duplicates-report - 只读巡检重复角色绑定"
	@echo "  make iam-role-binding-duplicates-fix    - 显式修复重复角色绑定"
