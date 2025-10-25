# ===== Audit partitions =====

AUDIT_TOOL=backend/cmd/tools/audit_partitions

.PHONY: audit-partitions-parent
audit-partitions-parent:
	@echo ">> Ensure parent partitioned table (idempotent)"
	go run $(AUDIT_TOOL) -mode=ensure-parent

# 预建分区：默认过去1个月+未来2个月
.PHONY: audit-partitions-ensure
audit-partitions-ensure:
	@echo ">> Ensure monthly partitions"
	go run $(AUDIT_TOOL) -mode=ensure -past=$(PAST) -future=$(FUTURE)

# 丢弃旧分区：默认保留6个月
.PHONY: audit-partitions-retire
audit-partitions-retire:
	@echo ">> Retire partitions older than retention window"
	go run $(AUDIT_TOOL) -mode=retire -retention=$(RETENTION)

# 例子：
# make audit-partitions-parent
# make audit-partitions-ensure PAST=2 FUTURE=3
# make audit-partitions-retire RETENTION=9
