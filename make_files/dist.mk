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
	mkdir -p "$(DIST_OUT_DIR)/backend" "$(DIST_OUT_DIR)/runner" "$(DIST_OUT_DIR)/web-admin" "$(DIST_OUT_DIR)/systemd" "$(DIST_OUT_DIR)/config"; \
	echo "[dist] build backend binary"; \
	go build -o "$(DIST_OUT_DIR)/backend/powerx" ./backend/cmd/app; \
	echo "[dist] copy backend runtime config"; \
	mkdir -p "$(DIST_OUT_DIR)/backend/etc"; \
	cp -R backend/etc/* "$(DIST_OUT_DIR)/backend/etc/"; \
	echo "[dist] build runner"; \
	if [ "$(NPM_INSTALL)" = "1" ]; then (cd backend/runner && npm ci); fi; \
	(cd backend/runner && npm run build); \
	cp -R backend/runner/dist "$(DIST_OUT_DIR)/runner/"; \
	echo "[dist] build web-admin"; \
	if [ "$(NPM_INSTALL)" = "1" ]; then (cd web-admin && npm ci); fi; \
	(cd web-admin && npm run build); \
	cp -R web-admin/.output "$(DIST_OUT_DIR)/web-admin/"; \
	echo "[dist] copy systemd units"; \
	cp deploy/powerx/systemd/*.service "$(DIST_OUT_DIR)/systemd/"; \
	echo "[dist] write env template"; \
	{ \
		echo "POWERX_ENV=prod"; \
		echo "POWERX_MODE=systemd"; \
		echo "DATABASE_DSN=postgres://powerx:powerx@127.0.0.1:5432/powerx?sslmode=disable"; \
		echo "REDIS_ADDR=127.0.0.1:6379"; \
	} > "$(DIST_OUT_DIR)/config/powerx.env.example"; \
	echo "version=$(DIST_VERSION)" > "$(DIST_OUT_DIR)/manifest.txt"; \
	echo "commit=$$(git rev-parse HEAD 2>/dev/null || echo unknown)" >> "$(DIST_OUT_DIR)/manifest.txt"; \
	echo "built_at=$$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$(DIST_OUT_DIR)/manifest.txt"; \
	echo "[dist] done"

dist-clean:
	@rm -rf "$(DIST_BASE_DIR)"
	@echo "[dist] cleaned: $(DIST_BASE_DIR)"
