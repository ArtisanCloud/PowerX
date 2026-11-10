<template>
  <section class="min-w-0 space-y-4" data-testid="cleanup-resolution">
    <div>
      <h3 class="font-semibold">{{ t("backupResolution.title") }}</h3>
      <p class="mt-2 text-sm text-muted">{{ t("backupResolution.hint") }}</p>
    </div>
    <UAlert v-if="error" color="warning" :title="error" />
    <UAlert v-if="loadError" color="warning" :title="loadError" />
    <UAlert
      v-if="unknown"
      color="warning"
      :title="t('backupResolution.unknown')"
    />
    <UAlert
      v-if="events.status.value === 'disconnected'"
      color="warning"
      :title="t('backupResolution.disconnected')"
    />
    <div
      v-if="receipt"
      class="space-y-2 rounded-xl border border-default p-4"
      data-testid="resolution-receipt"
    >
      <UBadge :color="receipt.state === 'retained' ? 'success' : 'warning'">{{
        t(
          "backupResolution.states." +
            (receipt.state === "checking" && receipt.recovery_blocked
              ? "blocked"
              : receipt.state),
        )
      }}</UBadge>
      <p class="break-all font-mono text-xs">{{ receipt.request_id }}</p>
      <p class="text-sm text-muted">{{ t("backupResolution.historical") }}</p>
      <p
        v-if="receipt.sql_committed && !receipt.independent_complete"
        class="text-sm"
      >
        {{ t("backupResolution.checkpoint") }}
      </p>
      <details v-if="receipt.problem" class="text-xs text-muted">
        <summary>{{ t("backupResolution.details") }}</summary>
        <p class="mt-2 break-all">{{ receipt.problem }}</p>
      </details>
      <UButton
        v-if="!pending && preview?.can_resolve && fresh && !disabled"
        color="neutral"
        variant="outline"
        data-testid="resolution-new-review"
        @click="newReview"
        >{{ t("backupResolution.newReview") }}</UButton
      >
    </div>
    <div v-if="running" role="status" class="space-y-2">
      <p class="text-sm">
        {{ t("backupResolution.running") }} · {{ progress?.bytes_read || 0 }} /
        {{ progress?.total_bytes || item?.size_bytes || 0 }} B
      </p>
      <progress
        class="h-2 w-full accent-primary"
        :value="progress?.bytes_read || 0"
        :max="Math.max(progress?.total_bytes || item?.size_bytes || 1, 1)"
      />
    </div>
    <div v-if="pending" class="space-y-3 rounded-xl border border-default p-4">
      <p class="break-all font-mono text-xs" data-testid="resolution-frozen-id">
        {{ pending.request_id }}
      </p>
      <p class="text-sm text-muted">{{ t("backupResolution.originalOnly") }}</p>
      <div class="flex flex-wrap gap-2">
        <UButton
          color="neutral"
          variant="outline"
          :disabled="running || reconciling"
          data-testid="resolution-reconcile"
          @click="reconcile"
          >{{ t("backupResolution.reconcile") }}</UButton
        ><UButton
          v-if="unknown && (!receipt || receipt.can_resume)"
          color="neutral"
          variant="outline"
          :disabled="running || reconciling"
          data-testid="resolution-resume"
          @click="submit(pending)"
          >{{ t("backupResolution.resume") }}</UButton
        ><UButton
          v-if="running"
          color="neutral"
          variant="outline"
          @click="stop"
          >{{ t("backupResolution.stop") }}</UButton
        >
      </div>
    </div>
    <template v-else-if="!receipt">
      <UAlert
        v-if="preview && !preview.can_resolve"
        color="warning"
        :title="t('backupResolution.blocked')"
        :description="preview.problem"
      />
      <p v-if="preview" class="text-sm text-muted">
        {{
          t("backupResolution.unknownCount", { count: preview.unknown_records })
        }}
      </p>
      <UFormField :label="t('backupResolution.reason')"
        ><UTextarea
          v-model="reason"
          :rows="3"
          :maxlength="512"
          :disabled="disabled || loading"
          class="w-full"
          data-testid="resolution-reason"
      /></UFormField>
      <UCheckbox
        v-model="confirmed"
        :disabled="disabled || loading"
        :label="t('backupResolution.confirm')"
        data-testid="resolution-confirm"
      />
      <UButton
        :disabled="
          disabled ||
          loading ||
          !fresh ||
          !preview?.can_resolve ||
          !confirmed ||
          [...reason.trim()].length < 3 ||
          [...reason.trim()].length > 256
        "
        data-testid="resolution-submit"
        @click="begin"
        >{{ t("backupResolution.submit") }}</UButton
      >
    </template>
    <UButton
      v-if="!running && !pending"
      color="neutral"
      variant="outline"
      :loading="loading"
      data-testid="resolution-refresh"
      @click="refresh"
      >{{ t("backupResolution.refresh") }}</UButton
    >
  </section>
</template>
<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount } from "vue";
import { useI18n } from "vue-i18n";
import type {
  BackupCleanupReview,
  BackupCleanupVerification,
} from "~/composables/api/services/backupCleanupReviewService";
import {
  useBackupResolutionService,
  type BackupResolutionInput,
  type BackupResolutionReceipt,
  type BackupResolutionPreview,
} from "~/composables/api/services/backupResolutionService";
import { useBackupEvents } from "~/composables/useBackupEvents";
import { AUTH_SESSION_EVENT, readAuthSession } from "~/utils/auth-session";
const props = defineProps<{ item?: BackupCleanupReview; disabled?: boolean }>(),
  emit = defineEmits<{ locked: [value: boolean]; settled: [value: string] }>();
const { t } = useI18n(),
  api = useBackupResolutionService();
const preview = ref<BackupResolutionPreview>(),
  receipt = ref<BackupResolutionReceipt>(),
  pending = ref<BackupResolutionInput>(),
  progress = ref<BackupCleanupVerification>();
const reason = ref(""),
  confirmed = ref(false),
  error = ref(""),
  loadError = ref(""),
  unknown = ref(false),
  running = ref(false),
  loading = ref(false),
  reconciling = ref(false),
  fresh = ref(false),
  reviewingNew = ref(false);
let controller: AbortController | undefined,
  loadController: AbortController | undefined,
  readController: AbortController | undefined,
  disposed = false;
const identity = () => {
  const s = readAuthSession();
  return `${s?.sessionId || ""}:${s?.tenantUuid || ""}:${s?.status || ""}`;
};
let owner = identity();
watch(
  [pending, running],
  () => emit("locked", !!pending.value || running.value),
  { immediate: true },
);
function accept(r: BackupResolutionReceipt) {
  receipt.value = r;
  if (r.state !== "checking" || r.recovery_blocked) {
    pending.value = undefined;
    unknown.value = false;
    running.value = false;
    confirmed.value = false;
    controller?.abort();
    if (r.independent_complete) {
      loadError.value = "";
      fresh.value = false;
    }
    if (r.state === "retained") emit("settled", "retained");
  } else if (r.can_resume && !pending.value) {
    pending.value = {
      request_id: r.request_id,
      expected_token: r.token,
      review_version: r.review_version,
      history_digest: r.history_digest,
      reason: r.reason,
      confirm: true,
    };
    unknown.value = true;
  }
}
async function refresh() {
  if (!props.item) return;
  loadController?.abort();
  const c = (loadController = new AbortController());
  loading.value = true;
  try {
    const rows = await api.list(
      props.item.artifact_id,
      props.item.token,
      c.signal,
    );
    if (c.signal.aborted || disposed) return;
    if (pending.value) {
      const r = rows.find((r) => r.request_id === pending.value!.request_id);
      if (r) accept(r);
    } else if (!receipt.value && !reviewingNew.value && rows.length)
      accept(rows[0]);
    const v = await api.preview(
      props.item.artifact_id,
      props.item.token,
      c.signal,
    );
    if (!c.signal.aborted && !disposed) {
      preview.value = v;
      fresh.value = true;
      loadError.value = "";
    }
  } catch {
    if (!c.signal.aborted && !disposed) {
      fresh.value = false;
      loadError.value =
        receipt.value?.state === "retained"
          ? ""
          : t("backupResolution.loadFailed");
    }
  } finally {
    if (!c.signal.aborted && !disposed) loading.value = false;
  }
}
function begin() {
  if (
    !props.item ||
    props.disabled ||
    !preview.value?.can_resolve ||
    !fresh.value ||
    !confirmed.value ||
    pending.value
  )
    return;
  void submit({
    request_id: crypto.randomUUID(),
    expected_token: props.item.token,
    review_version: preview.value.review_version,
    history_digest: preview.value.history_digest,
    reason: reason.value.trim(),
    confirm: true,
  });
}
async function submit(input: BackupResolutionInput) {
  if (!props.item || running.value) return;
  reviewingNew.value = false;
  pending.value = { ...input };
  running.value = true;
  unknown.value = false;
  error.value = "";
  progress.value = undefined;
  const c = (controller = new AbortController());
  try {
    const r = await api.resolve(
      props.item.artifact_id,
      { ...input },
      c.signal,
      (v) => {
        if (!c.signal.aborted && !disposed) {
          if (v.receipt) receipt.value = v.receipt;
          if (v.progress) progress.value = v.progress;
        }
      },
    );
    if (!c.signal.aborted && !disposed) accept(r);
  } catch {
    if (!disposed && !c.signal.aborted && pending.value) unknown.value = true;
  } finally {
    if (!disposed && controller === c) running.value = false;
  }
}
async function reconcile() {
  if (!pending.value) return;
  readController?.abort();
  const c = (readController = new AbortController());
  reconciling.value = true;
  try {
    const r = await api.request(pending.value.request_id, c.signal);
    if (!c.signal.aborted && !disposed) {
      accept(r);
      error.value = "";
    }
  } catch {
    if (!c.signal.aborted && !disposed)
      error.value = t("backupResolution.noReceipt");
  } finally {
    if (!disposed && readController === c) reconciling.value = false;
  }
}
function stop() {
  controller?.abort();
  running.value = false;
  unknown.value = true;
}
function newReview() {
  if (pending.value || !preview.value?.can_resolve || !fresh.value) return;
  reviewingNew.value = true;
  receipt.value = undefined;
  reason.value = "";
  confirmed.value = false;
  error.value = "";
  void refresh();
}
function identityChanged() {
  if (owner === identity()) return;
  owner = identity();
  controller?.abort();
  loadController?.abort();
  readController?.abort();
  reviewingNew.value = false;
  preview.value = undefined;
  receipt.value = undefined;
  pending.value = undefined;
  reason.value = "";
  confirmed.value = false;
  fresh.value = false;
  unknown.value = false;
  running.value = false;
  reconciling.value = false;
  error.value = loadError.value = "";
}
const events = useBackupEvents("backup_cleanup", () => void refresh());
onMounted(() => {
  void refresh();
  window.addEventListener(AUTH_SESSION_EVENT, identityChanged);
  window.addEventListener("storage", identityChanged);
});
onBeforeUnmount(() => {
  disposed = true;
  controller?.abort();
  loadController?.abort();
  readController?.abort();
  window.removeEventListener(AUTH_SESSION_EVENT, identityChanged);
  window.removeEventListener("storage", identityChanged);
  emit("locked", false);
});
</script>
