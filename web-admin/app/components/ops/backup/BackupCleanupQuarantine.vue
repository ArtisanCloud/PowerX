<template>
  <section class="min-w-0 space-y-4" data-testid="cleanup-quarantine">
    <div>
      <h3 class="font-semibold">{{ t("backupQuarantine.title") }}</h3>
      <p class="mt-2 text-sm text-muted">{{ t("backupQuarantine.hint") }}</p>
    </div>
    <UAlert v-if="error" color="warning" :title="error" />
    <UAlert v-if="loadError" color="warning" :title="loadError" />
    <UAlert
      v-if="unknown"
      color="warning"
      :title="t('backupQuarantine.unknown')"
    />
    <UAlert
      v-if="events.status.value === 'disconnected'"
      color="warning"
      :title="t('backupQuarantine.disconnected')"
    />
    <div
      v-if="receipt"
      class="space-y-2 rounded-xl border border-default p-4"
      data-testid="quarantine-receipt"
    >
      <UBadge
        :color="receipt.state === 'quarantined' ? 'success' : 'warning'"
        >{{
          t(
            "backupQuarantine.states." +
              (receipt.state === "checking" && receipt.recovery_blocked
                ? "blocked"
                : receipt.state),
          )
        }}</UBadge
      >
      <p class="break-all font-mono text-xs">{{ receipt.request_id }}</p>
      <p class="text-sm text-muted">{{ t("backupQuarantine.historical") }}</p>
      <p
        v-if="receipt.sql_committed && !receipt.independent_complete"
        class="text-sm"
      >
        {{ t("backupQuarantine.checkpoint") }}
      </p>
      <details v-if="receipt.problem" class="text-xs text-muted">
        <summary>{{ t("backupQuarantine.details") }}</summary>
        <p class="mt-2 break-all">{{ receipt.problem }}</p>
      </details>
      <UButton
        v-if="!pending && preview?.can_quarantine && fresh && !disabled"
        color="neutral"
        variant="outline"
        data-testid="quarantine-new-review"
        @click="newReview"
        >{{ t("backupQuarantine.newReview") }}</UButton
      >
    </div>
    <div v-if="running || diagnosing" role="status" class="space-y-2">
      <p class="text-sm">
        {{ t("backupQuarantine.running") }} · {{ progress?.bytes_read || 0 }} /
        {{ progress?.total_bytes || item?.size_bytes || 0 }} B
      </p>
      <progress
        class="h-2 w-full accent-primary"
        :value="progress?.bytes_read || 0"
        :max="Math.max(progress?.total_bytes || item?.size_bytes || 1, 1)"
      />
    </div>
    <div v-if="pending" class="space-y-3 rounded-xl border border-default p-4">
      <p class="break-all font-mono text-xs" data-testid="quarantine-frozen-id">
        {{ pending.request_id }}
      </p>
      <p class="text-sm text-muted">{{ t("backupQuarantine.originalOnly") }}</p>
      <div class="flex flex-wrap gap-2">
        <UButton
          color="neutral"
          variant="outline"
          :disabled="running || reconciling"
          data-testid="quarantine-reconcile"
          @click="reconcile"
          >{{ t("backupQuarantine.reconcile") }}</UButton
        ><UButton
          v-if="unknown && (!receipt || receipt.can_resume)"
          color="neutral"
          variant="outline"
          :disabled="running || reconciling"
          data-testid="quarantine-resume"
          @click="submit(pending)"
          >{{ t("backupQuarantine.resume") }}</UButton
        ><UButton
          v-if="running"
          color="neutral"
          variant="outline"
          @click="stop"
          >{{ t("backupQuarantine.stop") }}</UButton
        >
      </div>
    </div>
    <template v-else-if="!receipt">
      <UAlert
        v-if="preview?.diagnosis"
        color="warning"
        :title="t('backupReview.verify.states.' + preview.diagnosis)"
        :description="t('backupQuarantine.noProof')"
        data-testid="quarantine-diagnosis"
      />
      <UAlert
        v-if="preview && !preview.can_quarantine"
        color="warning"
        :title="t('backupQuarantine.blocked')"
        :description="preview.problem"
      />
      <p v-if="preview" class="text-sm text-muted">
        {{
          t("backupQuarantine.unknownCount", { count: preview.unknown_records })
        }}
      </p>
      <UFormField :label="t('backupQuarantine.reason')"
        ><UTextarea
          v-model="reason"
          :rows="3"
          :maxlength="512"
          :disabled="disabled || loading || diagnosing"
          class="w-full"
          data-testid="quarantine-reason"
      /></UFormField>
      <UCheckbox
        v-model="confirmed"
        :disabled="disabled || loading || diagnosing"
        :label="t('backupQuarantine.confirm')"
        data-testid="quarantine-confirm"
      />
      <UButton
        :disabled="
          disabled ||
          loading ||
          diagnosing ||
          !fresh ||
          !preview?.can_quarantine ||
          !confirmed ||
          [...reason.trim()].length < 3 ||
          [...reason.trim()].length > 256
        "
        data-testid="quarantine-submit"
        @click="begin"
        >{{ t("backupQuarantine.submit") }}</UButton
      >
    </template>
    <UButton
      v-if="!running && !pending && !disabled"
      color="neutral"
      variant="outline"
      :loading="diagnosing"
      :disabled="loading || diagnosing"
      data-testid="quarantine-refresh"
      @click="diagnose"
      >{{ t("backupQuarantine.refresh") }}</UButton
    >
    <UButton
      v-if="diagnosing"
      color="neutral"
      variant="outline"
      data-testid="quarantine-stop-diagnosis"
      @click="stopDiagnosis"
      >{{ t("backupQuarantine.stopDiagnosis") }}</UButton
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
  useBackupQuarantineService,
  type BackupQuarantineInput,
  type BackupQuarantineReceipt,
  type BackupQuarantinePreview,
} from "~/composables/api/services/backupQuarantineService";
import { useBackupEvents } from "~/composables/useBackupEvents";
import { AUTH_SESSION_EVENT, readAuthSession } from "~/utils/auth-session";
const props = defineProps<{ item?: BackupCleanupReview; disabled?: boolean }>(),
  emit = defineEmits<{ locked: [value: boolean]; settled: [value: string] }>();
const { t } = useI18n(),
  api = useBackupQuarantineService();
const preview = ref<BackupQuarantinePreview>(),
  receipt = ref<BackupQuarantineReceipt>(),
  pending = ref<BackupQuarantineInput>(),
  progress = ref<BackupCleanupVerification>();
const reason = ref(""),
  confirmed = ref(false),
  error = ref(""),
  loadError = ref(""),
  unknown = ref(false),
  running = ref(false),
  loading = ref(false),
  reconciling = ref(false),
  diagnosing = ref(false),
  fresh = ref(false),
  reviewingNew = ref(false);
let controller: AbortController | undefined,
  loadController: AbortController | undefined,
  readController: AbortController | undefined,
  diagnosisController: AbortController | undefined,
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
function accept(r: BackupQuarantineReceipt) {
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
    if (r.state === "quarantined") emit("settled", "quarantined");
  } else if (r.can_resume && !pending.value) {
    pending.value = {
      request_id: r.request_id,
      expected_token: r.token,
      review_version: r.review_version,
      history_digest: r.history_digest,
      reason: r.reason,
      confirm: true,
      expected_diagnosis: r.expected_diagnosis,
      diagnostic_digest: r.diagnostic_digest,
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
    loadError.value = "";
  } catch {
    if (!c.signal.aborted && !disposed) {
      fresh.value = false;
      loadError.value =
        receipt.value?.state === "quarantined"
          ? ""
          : t("backupQuarantine.loadFailed");
    }
  } finally {
    if (!c.signal.aborted && !disposed) loading.value = false;
  }
}
async function diagnose() {
  if (
    !props.item ||
    pending.value ||
    running.value ||
    diagnosing.value ||
    props.disabled
  )
    return;
  diagnosisController?.abort();
  const c = (diagnosisController = new AbortController());
  diagnosing.value = true;
  fresh.value = false;
  preview.value = undefined;
  progress.value = undefined;
  error.value = "";
  try {
    const v = await api.preview(
      props.item.artifact_id,
      props.item.token,
      c.signal,
      (p) => {
        if (!c.signal.aborted && !disposed) progress.value = p;
      },
    );
    if (!c.signal.aborted && !disposed) {
      preview.value = v;
      fresh.value = true;
    }
  } catch {
    if (!c.signal.aborted && !disposed)
      error.value = t("backupQuarantine.diagnosisIncomplete");
  } finally {
    if (!disposed && diagnosisController === c) diagnosing.value = false;
  }
}
function stopDiagnosis() {
  diagnosisController?.abort();
  diagnosing.value = false;
  fresh.value = false;
  error.value = t("backupQuarantine.diagnosisIncomplete");
}
function begin() {
  if (
    !props.item ||
    props.disabled ||
    !preview.value?.can_quarantine ||
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
    expected_diagnosis: preview.value.diagnosis,
    diagnostic_digest: preview.value.diagnostic_digest,
  });
}
async function submit(input: BackupQuarantineInput) {
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
      error.value = t("backupQuarantine.noReceipt");
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
  if (pending.value || !preview.value?.can_quarantine || !fresh.value) return;
  reviewingNew.value = true;
  receipt.value = undefined;
  reason.value = "";
  confirmed.value = false;
  error.value = "";
  void diagnose();
}
function identityChanged() {
  if (owner === identity()) return;
  owner = identity();
  controller?.abort();
  loadController?.abort();
  readController?.abort();
  diagnosisController?.abort();
  diagnosing.value = false;
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
const events = useBackupEvents("backup_cleanup", () => {
  fresh.value = false;
  if (diagnosing.value) stopDiagnosis();
  void refresh();
});
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
  diagnosisController?.abort();
  window.removeEventListener(AUTH_SESSION_EVENT, identityChanged);
  window.removeEventListener("storage", identityChanged);
  emit("locked", false);
});
</script>
