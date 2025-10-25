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
.PHONY: db-migrate
db-migrate:
	@echo "执行数据库迁移..."
	@go run ./backend/cmd/database migrate

.PHONY: db-rollback
db-rollback:
	@echo "执行数据库回滚..."
	@go run ./backend/cmd/database rollback

.PHONY: db-seed
db-seed:
	@echo "填充数据库种子数据..."
	@go run ./backend/cmd/database seed

.PHONY: db-refresh
db-refresh:
	@echo "刷新数据库（回滚+迁移+种子）..."
	@go run ./backend/cmd/database refresh

.PHONY: db-status
db-status:
	@echo "查看数据库迁移状态..."
	@go run ./backend/cmd/database status


# 帮助信息
.PHONY: help
help:
	@echo "数据库操作命令:"
	@echo "  make db-migrate    - 执行数据库迁移"
	@echo "  make db-rollback   - 回滚数据库迁移"
	@echo "  make db-seed       - 填充数据库种子数据"
	@echo "  make db-refresh    - 刷新数据库（回滚+迁移+种子）"
	@echo "  make db-status     - 查看数据库迁移状态"
	@echo "  make db-check      - 检查数据库是否存在"
