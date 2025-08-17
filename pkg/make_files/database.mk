# 数据库操作 Makefile

# 默认配置文件路径
CONFIG_PATH ?= config/config.yaml

# 数据库迁移
.PHONY: db-migrate
db-migrate:
	@echo "执行数据库迁移..."
	@go run cmd/db/main.go -op migrate

# 数据库回滚
.PHONY: db-rollback
db-rollback:
	@echo "执行数据库回滚..."
	@go run cmd/db/main.go -op rollback

# 数据库种子数据填充
.PHONY: db-seed
db-seed:
	@echo "填充数据库种子数据..."
	@go run cmd/db/main.go -op seed

# 数据库刷新（回滚+迁移+种子）
.PHONY: db-refresh
db-refresh:
	@echo "刷新数据库（回滚+迁移+种子）..."
	@go run cmd/db/main.go -op refresh

# 数据库状态
.PHONY: db-status
db-status:
	@echo "查看数据库迁移状态..."
	@go run cmd/db/main.go -op status

# 检查数据库是否存在
.PHONY: db-check
db-check:
	@echo "检查数据库是否存在..."
	@psql -U michaelhu -h localhost -p 5432 -c "SELECT 1 FROM pg_database WHERE datname='corex'" postgres

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
