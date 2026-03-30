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
		if [ -f backend/etc/config_example.prod.yaml ]; then \
			cp backend/etc/config_example.prod.yaml "$(DIST_OUT_DIR)/backend/etc/config.yaml"; \
		elif [ -f backend/etc/config_example.yaml ]; then \
			cp backend/etc/config_example.yaml "$(DIST_OUT_DIR)/backend/etc/config.yaml"; \
		fi; \
		if [ -d backend/config ]; then \
			mkdir -p "$(DIST_OUT_DIR)/backend/config"; \
			cp -R backend/config/* "$(DIST_OUT_DIR)/backend/config/"; \
		fi; \
	if [ -d config/plugins ]; then \
		mkdir -p "$(DIST_OUT_DIR)/backend/config/plugins"; \
		cp -R config/plugins/* "$(DIST_OUT_DIR)/backend/config/plugins/"; \
	fi; \
	if [ -d config/security ]; then \
		mkdir -p "$(DIST_OUT_DIR)/backend/config/security"; \
		cp -R config/security/* "$(DIST_OUT_DIR)/backend/config/security/"; \
	fi; \
	echo "[dist] copy backend runtime assets"; \
	if [ -d backend/internal/server/agent/blueprints ]; then \
		mkdir -p "$(DIST_OUT_DIR)/backend/internal/server/agent"; \
		cp -R backend/internal/server/agent/blueprints "$(DIST_OUT_DIR)/backend/internal/server/agent/"; \
	fi; \
	if [ -d backend/pkg/corex/flow/blueprints ]; then \
		mkdir -p "$(DIST_OUT_DIR)/backend/pkg/corex/flow"; \
		cp -R backend/pkg/corex/flow/blueprints "$(DIST_OUT_DIR)/backend/pkg/corex/flow/"; \
	fi; \
	CFG_FILE="$(DIST_OUT_DIR)/backend/etc/config.yaml"; \
		if [ -f "$$CFG_FILE" ]; then \
		if grep -q '^version:' "$$CFG_FILE"; then \
			awk -v v="$(DIST_VERSION)" 'BEGIN{done=0} { if (!done && $$0 ~ /^version:[[:space:]]*/) { print "version: " v; done=1; next } print }' "$$CFG_FILE" > "$$CFG_FILE.tmp"; \
			mv "$$CFG_FILE.tmp" "$$CFG_FILE"; \
		else \
			{ echo "version: $(DIST_VERSION)"; cat "$$CFG_FILE"; } > "$$CFG_FILE.tmp"; \
			mv "$$CFG_FILE.tmp" "$$CFG_FILE"; \
		fi; \
		SECRET_KEY="$$(head -c 32 /dev/urandom | base64 | tr -d '\r\n')"; \
		awk -v sk="$$SECRET_KEY" '\
			BEGIN { in_server=0; server_found=0; secret_found=0; child_indent="    " } \
			/^server:[[:space:]]*$$/ { \
				server_found=1; in_server=1; print; next \
			} \
			{ \
				if (in_server && $$0 ~ /^[[:space:]]+[^[:space:]#][^:]*:[[:space:]]*/ ) { \
					match($$0, /^[[:space:]]+/); \
					if (RSTART > 0 && RLENGTH > 0) child_indent=substr($$0, RSTART, RLENGTH); \
				} \
				if (in_server && $$0 ~ /^[^[:space:]]/ ) { \
					if (!secret_found) print child_indent "secret_key: \"" sk "\""; \
					in_server=0; \
				} \
				if (in_server && $$0 ~ /^[[:space:]]+secret_key:[[:space:]]*/) { \
					secret_found=1; \
					match($$0, /^[[:space:]]*/); \
					indent=substr($$0, RSTART, RLENGTH); \
					val=$$0; sub(/^[[:space:]]*secret_key:[[:space:]]*/, "", val); gsub(/[[:space:]]/, "", val); \
					if (val == "" || val == "\"\"" || val == "\047\047") { \
						print indent "secret_key: \"" sk "\""; \
					} else { \
						print; \
					} \
					next; \
				} \
				print; \
			} \
			END { \
				if (in_server && !secret_found) print child_indent "secret_key: \"" sk "\""; \
				if (!server_found) { \
					print ""; \
					print "server:"; \
					print "  secret_key: \"" sk "\""; \
				} \
			}' "$$CFG_FILE" > "$$CFG_FILE.tmp"; \
		mv "$$CFG_FILE.tmp" "$$CFG_FILE"; \
			awk '{ gsub(/\.\.\/config\/plugins\//, "./config/plugins/"); gsub(/\.\.\/config\/security\//, "./config/security/"); print }' "$$CFG_FILE" > "$$CFG_FILE.tmp"; \
			mv "$$CFG_FILE.tmp" "$$CFG_FILE"; \
			awk '\
				BEGIN { in_install=0; install_found=0; status_replaced=0 } \
				/^install:[[:space:]]*$$/ { in_install=1; install_found=1; print; next } \
				{ \
					if (in_install && $$0 ~ /^[^[:space:]]/) { \
						if (!status_replaced) print "  status: uninstalled"; \
						in_install=0; \
					} \
					if (in_install && $$0 ~ /^[[:space:]]+status:[[:space:]]*/) { \
						print "  status: uninstalled"; \
						status_replaced=1; \
						next; \
					} \
					print; \
				} \
				END { \
					if (in_install && !status_replaced) print "  status: uninstalled"; \
					if (!install_found) { \
						print ""; \
						print "install:"; \
						print "  status: uninstalled"; \
						print "  lock_mode: strict"; \
						print "  allow_without_db: true"; \
					} \
				}' "$$CFG_FILE" > "$$CFG_FILE.tmp"; \
			mv "$$CFG_FILE.tmp" "$$CFG_FILE"; \
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
	(cd web-admin && POWERX_ENV=prod POWERX_BUILD_TARGET=prod UPSTREAM=http://127.0.0.1:8080 WS_UPSTREAM=ws://127.0.0.1:8080/api/ws npm run build); \
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
			echo "POWERX_CONFIG=/opt/powerx/backend/etc/config.yaml"; \
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
			if grep -q '^POWERX_BACKEND_PORT=' "$$ENV_FILE"; then \
				awk '{ if ($$0 ~ /^POWERX_BACKEND_PORT=/) { print "POWERX_BACKEND_PORT=8080" } else { print } }' "$$ENV_FILE" > "$$ENV_FILE.tmp"; \
				mv "$$ENV_FILE.tmp" "$$ENV_FILE"; \
			else \
				echo "POWERX_BACKEND_PORT=8080" >> "$$ENV_FILE"; \
			fi; \
			if grep -q '^POWERX_GRPC_PORT=' "$$ENV_FILE"; then \
				awk '{ if ($$0 ~ /^POWERX_GRPC_PORT=/) { print "POWERX_GRPC_PORT=9010" } else { print } }' "$$ENV_FILE" > "$$ENV_FILE.tmp"; \
				mv "$$ENV_FILE.tmp" "$$ENV_FILE"; \
			else \
				echo "POWERX_GRPC_PORT=9010" >> "$$ENV_FILE"; \
			fi; \
			if grep -q '^POWERX_CONFIG=' "$$ENV_FILE"; then \
				awk '{ if ($$0 ~ /^POWERX_CONFIG=/) { print "POWERX_CONFIG=/opt/powerx/backend/etc/config.yaml" } else { print } }' "$$ENV_FILE" > "$$ENV_FILE.tmp"; \
				mv "$$ENV_FILE.tmp" "$$ENV_FILE"; \
			else \
				echo "POWERX_CONFIG=/opt/powerx/backend/etc/config.yaml" >> "$$ENV_FILE"; \
			fi; \
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
		WEB_ENV_FILE="$(DIST_OUT_DIR)/config/web-admin.env"; \
		if [ -f "$$WEB_ENV_FILE" ]; then \
			if grep -q '^UPSTREAM=' "$$WEB_ENV_FILE"; then \
				awk '{ if ($$0 ~ /^UPSTREAM=/) { print "UPSTREAM=http://127.0.0.1:8080" } else { print } }' "$$WEB_ENV_FILE" > "$$WEB_ENV_FILE.tmp"; \
				mv "$$WEB_ENV_FILE.tmp" "$$WEB_ENV_FILE"; \
			else \
				echo "UPSTREAM=http://127.0.0.1:8080" >> "$$WEB_ENV_FILE"; \
			fi; \
			if grep -q '^WS_UPSTREAM=' "$$WEB_ENV_FILE"; then \
				awk '{ if ($$0 ~ /^WS_UPSTREAM=/) { print "WS_UPSTREAM=ws://127.0.0.1:8080/api/ws" } else { print } }' "$$WEB_ENV_FILE" > "$$WEB_ENV_FILE.tmp"; \
				mv "$$WEB_ENV_FILE.tmp" "$$WEB_ENV_FILE"; \
			else \
				echo "WS_UPSTREAM=ws://127.0.0.1:8080/api/ws" >> "$$WEB_ENV_FILE"; \
			fi; \
			cp "$$WEB_ENV_FILE" "$(DIST_OUT_DIR)/config/web-admin.env.example"; \
		fi; \
		echo "version=$(DIST_VERSION)" > "$(DIST_OUT_DIR)/manifest.txt"; \
	echo "commit=$$(git rev-parse HEAD 2>/dev/null || echo unknown)" >> "$(DIST_OUT_DIR)/manifest.txt"; \
	echo "built_at=$$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$(DIST_OUT_DIR)/manifest.txt"; \
	echo "[dist] done"

dist-clean:
	@rm -rf "$(DIST_BASE_DIR)"
	@echo "[dist] cleaned: $(DIST_BASE_DIR)"
