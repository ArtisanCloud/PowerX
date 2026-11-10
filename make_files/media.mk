# 显式历史补图维护；不得作为 migrate、启动或上传目标的依赖。
CONFIG ?= backend/etc/config.yaml
TENANT_UUID ?=
REPORT ?=
PLAN ?=
PROGRESS ?=
BATCH_SIZE ?= 50
CONCURRENCY ?= 2
LIMIT ?= 0
AFTER_ID ?= 0
THROUGH_ID ?= 0
ASSETS ?=
RETRY_FAILED ?= 0
CONFIRM ?= 0
MEDIA_TOOL ?=

.PHONY: media-derivatives-scan media-derivatives-apply

# Pass values through the environment, not interpolated shell code. The thin
# launcher resolves paths from the repository root before entering backend.
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_CONFIG = $(CONFIG)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_TENANT = $(TENANT_UUID)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_REPORT = $(REPORT)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_PLAN = $(PLAN)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_PROGRESS = $(PROGRESS)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_BATCH = $(BATCH_SIZE)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_CONCURRENCY = $(CONCURRENCY)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_LIMIT = $(LIMIT)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_AFTER = $(AFTER_ID)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_THROUGH = $(THROUGH_ID)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_ASSETS = $(ASSETS)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_RETRY = $(RETRY_FAILED)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_CONFIRM = $(CONFIRM)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_TOOL = $(MEDIA_TOOL)
media-derivatives-scan media-derivatives-apply: export MEDIA_MAINT_GO = $(GO)

media-derivatives-scan:
	@sh scripts/ops/media-derivatives.sh scan

media-derivatives-apply:
	@sh scripts/ops/media-derivatives.sh apply

# 已知 PNG 校验异常的独立维护目标；不使用通用补图任务重试损坏原图。
.PHONY: media-png-checksum-scan media-png-checksum-apply
media-png-checksum-scan:
	@test -n "$(REPORT)" || (echo MEDIA_PNG_REPAIR_REPORT_REQUIRED; exit 1)
	cd backend && GOWORK=off go run ./cmd/png-checksum-repair --config "$(abspath $(CONFIG))" --tenant "$(TENANT_UUID)" --asset "$(ASSETS)" --report "$(abspath $(REPORT))"

media-png-checksum-apply:
	@test "$(CONFIRM)" = "1" || (echo MEDIA_PNG_REPAIR_CONFIRM_REQUIRED; exit 1)
	@test -n "$(PLAN)" -a -n "$(REPORT)" || (echo MEDIA_PNG_REPAIR_PATH_REQUIRED; exit 1)
	cd backend && GOWORK=off go run ./cmd/png-checksum-repair --config "$(abspath $(CONFIG))" --apply --confirm --plan "$(abspath $(PLAN))" --report "$(abspath $(REPORT))"
