.PHONY: capability-audit capability-check capability-seed

CAPABILITY_AUDIT_SCAN ?=
CAPABILITY_AUDIT_REQUIRED ?=
CAPABILITY_AUDIT_CANDIDATE_FILE ?=
CAPABILITY_AUDIT_FIX ?= 0
CAPABILITY_AUDIT_FIX_OUTPUT ?= tmp/capability-audit/missing.platform-capabilities.yaml
CAPABILITY_CHECK_GENERATED ?= tmp/capability-check/generated.platform-capabilities.yaml
CAPABILITY_SEED_CONFIG ?= $(if $(POWERX_CONFIG),$(POWERX_CONFIG),etc/config.yaml)

capability-audit:
	cd backend && CAPABILITY_AUDIT_SCAN="$(CAPABILITY_AUDIT_SCAN)" CAPABILITY_AUDIT_REQUIRED="$(CAPABILITY_AUDIT_REQUIRED)" CAPABILITY_AUDIT_CANDIDATE_FILE="$(CAPABILITY_AUDIT_CANDIDATE_FILE)" CAPABILITY_AUDIT_FIX="$(CAPABILITY_AUDIT_FIX)" $(GO) run ./cmd/capability_audit \
		-repo-root .. \
		-platform-dir backend/config/platform_capabilities \
		-required-file backend/config/capability_audit_required.yaml \
		-fix-output "$(CAPABILITY_AUDIT_FIX_OUTPUT)"

capability-check:
	$(MAKE) capability-gen CAPABILITY_GEN_OUT="../$(CAPABILITY_CHECK_GENERATED)"
	$(MAKE) capability-audit CAPABILITY_AUDIT_CANDIDATE_FILE="$(CAPABILITY_CHECK_GENERATED)"

capability-seed:
	cd backend && $(GO) run ./cmd/platform_capability_seed -config "$(CAPABILITY_SEED_CONFIG)"
