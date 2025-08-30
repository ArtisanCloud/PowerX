# pkg/make_files/secret.mk
# 生成/校验 WRAP_MASTER_KEY_ID / WRAP_MASTER_KEY_B64（32B after base64 decode）

SHELL := /bin/bash

ENV_FILE ?= .env.wrap
WRAP_ID  ?= v1
NS       ?= default
UNIT     ?= powerx.service

.PHONY: wrap-secrets wrap-rotate wrap-verify wrap-export wrap-print wrap-compose-env wrap-k8s-secret wrap-systemd-env wrap-clean

# 生成 32 字节 base64（优先 openssl，退回 /dev/urandom）
define GEN_B64
( command -v openssl >/dev/null 2>&1 && openssl rand -base64 32 ) || ( dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 )
endef

wrap-secrets:
	@if [ -f "$(ENV_FILE)" ]; then echo "ERROR: $(ENV_FILE) exists. Use 'make wrap-rotate'."; exit 1; fi
	@TS=$$(date -u +"%Y-%m-%dT%H:%M:%SZ"); KEY="$$( $(GEN_B64) | tr -d '\n' )"; \
	printf "# generated at %s UTC\nWRAP_MASTER_KEY_ID=%s\nWRAP_MASTER_KEY_B64=%s\n" "$$TS" "$(WRAP_ID)" "$$KEY" > "$(ENV_FILE)"; \
	echo "Wrote $(ENV_FILE)"; \
	$(MAKE) wrap-verify

wrap-rotate:
	@TS=$$(date -u +"%Y-%m-%dT%H:%M:%SZ"); KEY="$$( $(GEN_B64) | tr -d '\n' )"; \
	printf "# generated at %s UTC\nWRAP_MASTER_KEY_ID=%s\nWRAP_MASTER_KEY_B64=%s\n" "$$TS" "$(WRAP_ID)" "$$KEY" > "$(ENV_FILE)"; \
	echo "Rotated $(ENV_FILE)"; \
	$(MAKE) wrap-verify

wrap-verify:
	@KEY=$$(awk -F= '/^WRAP_MASTER_KEY_B64=/{print $$2}' "$(ENV_FILE)"); \
	if [ -z "$$KEY" ]; then echo "ERROR: WRAP_MASTER_KEY_B64 missing in $(ENV_FILE)"; exit 2; fi; \
	LEN=$$(echo -n "$$KEY" | openssl base64 -d 2>/dev/null | wc -c | tr -d ' '); \
	if [ "$$LEN" != "32" ]; then echo "LEN ERROR: $$LEN != 32"; exit 3; fi; \
	echo "OK: base64 decoded length = 32 bytes"

wrap-export:
	@awk 'BEGIN{FS="="} /^WRAP_MASTER_KEY_ID=|^WRAP_MASTER_KEY_B64=/{print "export "$$1"="$$2}' "$(ENV_FILE)"

wrap-print:
	@ID=$$(awk -F= '/^WRAP_MASTER_KEY_ID=/{print $$2}' "$(ENV_FILE)"); \
	KEY=$$(awk -F= '/^WRAP_MASTER_KEY_B64=/{print $$2}' "$(ENV_FILE)"); \
	[ -n "$$KEY" ] || { echo "No key found in $(ENV_FILE)"; exit 1; }; \
	L=$$(echo -n "$$KEY" | wc -c | tr -d ' '); \
	PFX=$$(echo -n "$$KEY" | cut -c1-4); \
	SFX=$$(echo -n "$$KEY" | cut -c$$(($$L-3))-$$L); \
	echo "WRAP_MASTER_KEY_ID=$$ID"; \
	echo "WRAP_MASTER_KEY_B64=$${PFX}****$${SFX} (len=$$L)"

wrap-compose-env:
	@if [ -f ".env" ]; then \
	  echo "WARN: .env already exists; not overwriting. Use 'cp $(ENV_FILE) .env' if desired."; \
	else \
	  cp "$(ENV_FILE)" .env && echo "Wrote .env (copied from $(ENV_FILE))"; \
	fi

wrap-k8s-secret:
	@ID=$$(awk -F= '/^WRAP_MASTER_KEY_ID=/{print $$2}' "$(ENV_FILE)"); \
	KEY=$$(awk -F= '/^WRAP_MASTER_KEY_B64=/{print $$2}' "$(ENV_FILE)"); \
	mkdir -p build/k8s; \
	cat > build/k8s/wrap-master-key-secret.yaml <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: wrap-master-key
  namespace: $(NS)
type: Opaque
stringData:
  WRAP_MASTER_KEY_ID: "$$ID"
  WRAP_MASTER_KEY_B64: "$$KEY"
EOF
	@echo "Wrote build/k8s/wrap-master-key-secret.yaml"

wrap-systemd-env:
	@ID=$$(awk -F= '/^WRAP_MASTER_KEY_ID=/{print $$2}' "$(ENV_FILE)"); \
	KEY=$$(awk -F= '/^WRAP_MASTER_KEY_B64=/{print $$2}' "$(ENV_FILE)"); \
	mkdir -p build/systemd; \
	cat > build/systemd/$(UNIT).conf <<EOF
[Service]
Environment="WRAP_MASTER_KEY_ID=$$ID"
Environment="WRAP_MASTER_KEY_B64=$$KEY"
EOF
	@echo "Wrote build/systemd/$(UNIT).conf"

wrap-clean:
	@rm -f "$(ENV_FILE)" .env
	@rm -rf build/k8s build/systemd
	@echo "Cleaned."
