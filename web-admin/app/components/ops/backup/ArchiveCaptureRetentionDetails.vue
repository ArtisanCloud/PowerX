<template>
  <details
    class="mt-3 rounded-lg border border-default p-3 text-sm"
    data-testid="archive-retention-audit"
  >
    <summary class="cursor-pointer font-medium">
      {{ t("storageGov.archive.disposition.audit") }}
    </summary>
    <dl class="mt-3 grid gap-3 sm:grid-cols-2">
      <div class="min-w-0 sm:col-span-2">
        <dt class="text-muted">
          {{ t("storageGov.archive.disposition.reason") }}
        </dt>
        <dd class="mt-1 whitespace-pre-wrap break-words">{{ value.reason }}</dd>
      </div>
      <div>
        <dt class="text-muted">
          {{ t("storageGov.archive.disposition.operator") }}
        </dt>
        <dd>{{ value.operator }}</dd>
      </div>
      <div>
        <dt class="text-muted">
          {{ t("storageGov.archive.disposition.recordedAt") }}
        </dt>
        <dd>{{ new Date(value.at).toLocaleString() }}</dd>
      </div>
      <div class="min-w-0 sm:col-span-2">
        <dt class="text-muted">
          {{ t("storageGov.archive.disposition.request") }}
        </dt>
        <dd class="mt-1 break-all font-mono text-xs">{{ value.request_id }}</dd>
      </div>
      <div class="sm:col-span-2">
        <dt class="text-muted">
          {{ t("storageGov.archive.disposition.sharedObjects") }}
        </dt>
        <dd>{{ value.object_count }} · {{ bytes(value.bytes) }}</dd>
      </div>
    </dl>
  </details>
</template>
<script setup lang="ts">
import { useI18n } from "vue-i18n";
import type { ArchiveCaptureRetention } from "~/composables/api/services/archiveOpsService";
import { formatStorageBytes as bytes } from "~/utils/storage-capacity";
defineProps<{ value: ArchiveCaptureRetention }>();
const { t } = useI18n();
</script>
