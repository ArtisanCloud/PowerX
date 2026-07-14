.PHONY: metadata-seed metadata-seed-validate

METADATA_SEED_CONFIG ?= backend/config/metadata_governance/seed.yaml
METADATA_SEED_TENANT_UUID ?=
POWERX_CONFIG ?= etc/config.yaml

metadata-seed:
	cd backend && go run ./cmd/metadata_seed -config "$(POWERX_CONFIG)" -tenant-uuid "$(METADATA_SEED_TENANT_UUID)" -seed "../$(METADATA_SEED_CONFIG)"

metadata-seed-validate:
	cd backend && go run ./cmd/metadata_seed -tenant-uuid "$(METADATA_SEED_TENANT_UUID)" -seed "../$(METADATA_SEED_CONFIG)" -dry-run -require-canonical
