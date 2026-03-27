# 发布打包（systemd）
# 目标：生成可直接部署到 /opt/powerx 的制品目录

DIST_VERSION ?= $(shell git describe --tags --always 2>/dev/null || date +%Y%m%d%H%M%S)
DIST_BASE_DIR ?= dist/systemd
DIST_OUT_DIR ?= $(DIST_BASE_DIR)/$(DIST_VERSION)

# 1 表示执行 npm ci；0 表示跳过安装，仅执行构建（适合已提前安装依赖）
NPM_INSTALL ?= 1

.PHONY: dist dist-systemd dist-clean

dist: dist-systemd

dist-systemd:
	@set -e; \
	echo "[dist] output: $(DIST_OUT_DIR)"; \
	RUNNER_DIR=""; \
	if [ -d backend/runner ]; then RUNNER_DIR="backend/runner"; fi; \
	if [ -z "$$RUNNER_DIR" ] && [ -d runner ]; then RUNNER_DIR="runner"; fi; \
	mkdir -p "$(DIST_OUT_DIR)/backend" "$(DIST_OUT_DIR)/runner" "$(DIST_OUT_DIR)/web-admin" "$(DIST_OUT_DIR)/systemd" "$(DIST_OUT_DIR)/config"; \
	echo "[dist] build backend binary"; \
	(cd backend && go build -o "../$(DIST_OUT_DIR)/backend/powerx" ./cmd/app); \
	echo "[dist] copy backend runtime config"; \
	mkdir -p "$(DIST_OUT_DIR)/backend/etc"; \
	cp -R backend/etc/* "$(DIST_OUT_DIR)/backend/etc/"; \
	CFG_FILE="$(DIST_OUT_DIR)/backend/etc/config.yaml"; \
	if [ -f "$$CFG_FILE" ]; then \
		if grep -q '^version:' "$$CFG_FILE"; then \
			awk -v v="$(DIST_VERSION)" 'BEGIN{done=0} { if (!done && $$0 ~ /^version:[[:space:]]*/) { print "version: " v; done=1; next } print }' "$$CFG_FILE" > "$$CFG_FILE.tmp"; \
			mv "$$CFG_FILE.tmp" "$$CFG_FILE"; \
		else \
			{ echo "version: $(DIST_VERSION)"; cat "$$CFG_FILE"; } > "$$CFG_FILE.tmp"; \
			mv "$$CFG_FILE.tmp" "$$CFG_FILE"; \
		fi; \
	fi; \
	if [ -n "$$RUNNER_DIR" ]; then \
		echo "[dist] build runner from $$RUNNER_DIR"; \
		if [ "$(NPM_INSTALL)" = "1" ]; then (cd "$$RUNNER_DIR" && npm ci); fi; \
		(cd "$$RUNNER_DIR" && npm run build); \
		cp -R "$$RUNNER_DIR"/dist "$(DIST_OUT_DIR)/runner/"; \
	else \
		echo "[dist] skip runner: backend/runner and runner not found"; \
	fi; \
	echo "[dist] build web-admin"; \
	if [ "$(NPM_INSTALL)" = "1" ]; then (cd web-admin && npm ci); fi; \
	(cd web-admin && npm run build); \
	cp -R web-admin/.output "$(DIST_OUT_DIR)/web-admin/"; \
	echo "[dist] copy systemd units"; \
	cp deploy/powerx/systemd/*.service "$(DIST_OUT_DIR)/systemd/"; \
	echo "[dist] copy env templates"; \
	if [ -f backend/.env.example ]; then \
		cp backend/.env.example "$(DIST_OUT_DIR)/config/powerx.env"; \
		cp backend/.env.example "$(DIST_OUT_DIR)/config/powerx.env.example"; \
	else \
		{ \
			echo "POWERX_ENV=prod"; \
			echo "POWERX_MODE=systemd"; \
			echo "POWERX_BACKEND_PORT=8080"; \
			echo "POWERX_WEB_ADMIN_PORT=3000"; \
			echo "POWERX_GRPC_PORT=9010"; \
			echo "DATABASE_DSN=postgres://powerx:powerx@127.0.0.1:5432/powerx?sslmode=disable"; \
			echo "REDIS_ADDR=127.0.0.1:6379"; \
		} > "$(DIST_OUT_DIR)/config/powerx.env"; \
		cp "$(DIST_OUT_DIR)/config/powerx.env" "$(DIST_OUT_DIR)/config/powerx.env.example"; \
	fi; \
	ENV_FILE="$(DIST_OUT_DIR)/config/powerx.env"; \
	if [ -f "$$ENV_FILE" ]; then \
		if grep -q '^POWERX_VERSION=' "$$ENV_FILE"; then \
			awk -v v="$(DIST_VERSION)" '{ if ($$0 ~ /^POWERX_VERSION=/) { print "POWERX_VERSION=" v } else { print } }' "$$ENV_FILE" > "$$ENV_FILE.tmp"; \
			mv "$$ENV_FILE.tmp" "$$ENV_FILE"; \
		else \
			echo "POWERX_VERSION=$(DIST_VERSION)" >> "$$ENV_FILE"; \
		fi; \
	fi; \
	cp "$(DIST_OUT_DIR)/config/powerx.env" "$(DIST_OUT_DIR)/config/powerx.env.example"; \
	if [ -f web-admin/.env.example ]; then \
		cp web-admin/.env.example "$(DIST_OUT_DIR)/config/web-admin.env"; \
		cp web-admin/.env.example "$(DIST_OUT_DIR)/config/web-admin.env.example"; \
	fi; \
	echo "version=$(DIST_VERSION)" > "$(DIST_OUT_DIR)/manifest.txt"; \
	echo "commit=$$(git rev-parse HEAD 2>/dev/null || echo unknown)" >> "$(DIST_OUT_DIR)/manifest.txt"; \
	echo "built_at=$$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$(DIST_OUT_DIR)/manifest.txt"; \
	echo "[dist] done"

dist-clean:
	@rm -rf "$(DIST_BASE_DIR)"
	@echo "[dist] cleaned: $(DIST_BASE_DIR)"
