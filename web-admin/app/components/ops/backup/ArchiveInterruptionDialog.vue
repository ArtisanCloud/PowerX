<template>
  <UModal
    v-model:open="opened"
    :dismissible="!busy && !pending"
    :title="
      t(
        closureMode
          ? 'storageGov.archive.disposition.closure.title'
          : 'storageGov.archive.disposition.title',
      )
    "
    :description="
      t(
        closureMode
          ? 'storageGov.archive.disposition.closure.reviewHint'
          : 'storageGov.archive.disposition.hint',
      )
    "
    :ui="{ content: 'max-w-3xl' }"
  >
    <template #body
      ><section
        class="min-w-0 space-y-4"
        data-testid="archive-disposition-dialog"
      >
        <UAlert
          v-if="connectionStatus === 'disconnected'"
          color="warning"
          variant="soft"
          :ui="{ title: 'text-highlighted', description: 'text-toned' }"
          :title="t('storageGov.archive.disconnected')"
          data-testid="archive-disposition-disconnected"
        />
        <UAlert
          v-if="
            job &&
            ['unconfirmed', 'legacy_unproven'].includes(
              job.command_drain_state || '',
            )
          "
          color="warning"
          variant="soft"
          :ui="{ title: 'text-highlighted', description: 'text-toned' }"
          :title="t('storageGov.archive.commands.unconfirmed')"
          :description="
            t('storageGov.archive.commands.protection', { count: 1 })
          "
          data-testid="archive-command-protection"
        />
        <UAlert
          v-if="contextError"
          color="error"
          :title="t('storageGov.archive.runRequest.contextUnavailable')"
          :description="contextError"
          data-testid="archive-disposition-context"
        />
        <UAlert
          v-if="error"
          color="error"
          :title="t('storageGov.requestFailed')"
          :description="error"
          data-testid="archive-disposition-error"
        />
        <UAlert
          v-if="pending"
          color="warning"
          variant="soft"
          :ui="{ title: 'text-highlighted', description: 'text-toned' }"
          :title="t('storageGov.archive.disposition.unknown')"
          :description="t('storageGov.archive.disposition.unknownHint')"
          data-testid="archive-disposition-pending"
          ><template #actions
            ><div class="min-w-0 space-y-2">
              <p class="break-all text-xs">{{ pending.request_id }}</p>
              <UButton
                :disabled="busy"
                color="neutral"
                variant="outline"
                data-testid="archive-disposition-reconcile"
                @click="reconcile"
                >{{ t("storageGov.archive.runRequest.reconcile") }}</UButton
              >
            </div></template
          ></UAlert
        >
        <UAlert
          v-if="result"
          :color="result.state === 'retained' ? 'success' : 'warning'"
          :title="
            t(
              result.state === 'closed_unknown'
                ? 'storageGov.archive.disposition.closure.closed'
                : result.state === 'retained'
                  ? 'storageGov.archive.disposition.retained'
                  : 'storageGov.archive.disposition.rejected',
            )
          "
          data-testid="archive-disposition-result"
        />
        <div
          v-if="result?.closure"
          class="rounded-lg border border-default p-3 text-sm space-y-1"
          data-testid="archive-closure-audit"
        >
          <p>{{ t("storageGov.archive.disposition.closure.closedHint") }}</p>
          <p class="whitespace-pre-wrap break-words">
            {{ result.closure.reason }}
          </p>
          <p>
            {{
              t("storageGov.archive.disposition.closure.actor", {
                id: result.closure.operator,
              })
            }}
            · {{ new Date(result.closure.at).toLocaleString() }}
          </p>
          <p class="break-all text-xs text-muted">
            {{ result.closure.request_id }}
          </p>
        </div>
        <p class="break-all text-xs text-muted">{{ effectiveId }}</p>
        <BackupSectionTabs
          v-model="section"
          :items="sections"
          :label="t('storageGov.archive.disposition.title')"
        >
          <template #overview
            ><div class="space-y-3">
              <p v-if="pending">
                {{ t("storageGov.archive.disposition.closure.reviewHint") }}
              </p>
              <p v-else>{{ t("storageGov.archive.disposition.scope") }}</p>
              <p v-if="job" class="text-sm">
                {{ job.source_id }} · {{ job.status }}
              </p>
              <ArchiveCaptureRetentionDetails
                v-if="job?.capture_retention"
                :value="job.capture_retention"
              />
              <p v-if="loading" role="status">{{ t("storageGov.loading") }}</p>
              <UButton
                v-if="pending"
                :disabled="busy || !!pending.closure || !closureAvailable"
                @click="section = 'requestReview'"
                >{{
                  t("storageGov.archive.disposition.closure.review")
                }}</UButton
              >
              <UButton
                v-else
                :disabled="busy || !!pending || !eligible"
                @click="section = 'verification'"
                >{{ t("storageGov.archive.disposition.startReview") }}</UButton
              >
              <p v-if="job && !eligible && !result" class="text-warning">
                {{ t("storageGov.archive.disposition.ineligible") }}
              </p>
            </div></template
          >
          <template #requestReview
            ><div class="space-y-4">
              <p>
                {{ t("storageGov.archive.disposition.closure.reviewHint") }}
              </p>
              <p v-if="!closureAvailable" class="text-warning">
                {{ t("storageGov.archive.disposition.closure.upgrade") }}
              </p>
              <div
                v-if="requestReview"
                class="rounded-lg border border-default p-3 space-y-2 text-sm"
                data-testid="archive-closure-reviewed"
              >
                <p>
                  {{ t("storageGov.archive.disposition.closure.protection") }}
                </p>
                <p>
                  {{
                    t(
                      requestReview.registered_epoch_matches
                        ? "storageGov.archive.disposition.closure.sameEpoch"
                        : "storageGov.archive.disposition.closure.changedEpoch",
                    )
                  }}
                </p>
                <p class="break-all text-xs text-muted">
                  {{ requestReview.input_digest }}
                </p>
              </div>
              <UButton
                :disabled="
                  busy || !pending || !!pending.closure || !closureAvailable
                "
                data-testid="archive-closure-review"
                @click="inspectRequest"
                >{{
                  t("storageGov.archive.disposition.closure.review")
                }}</UButton
              >
              <UButton
                color="neutral"
                variant="outline"
                :disabled="busy || !requestReview || !!pending?.closure"
                @click="section = 'requestClose'"
                >{{ t("storageGov.archive.disposition.next") }}</UButton
              >
            </div></template
          >
          <template #requestClose
            ><div class="space-y-4">
              <UAlert
                color="warning"
                variant="soft"
                :ui="{ title: 'text-highlighted', description: 'text-toned' }"
                :title="t('storageGov.archive.disposition.closure.effect')"
                :description="
                  t('storageGov.archive.disposition.closure.closedHint')
                "
              />
              <p
                v-if="pending?.closure"
                class="text-warning"
                data-testid="archive-closure-pending"
              >
                {{ t("storageGov.archive.disposition.closure.pending")
                }}<span class="block break-all text-xs">{{
                  pending.closure.request_id
                }}</span>
              </p>
              <UFormField :label="t('storageGov.archive.disposition.reason')"
                ><UTextarea
                  v-model="closureReason"
                  :rows="3"
                  :maxlength="256"
                  :disabled="busy || !!pending?.closure || !pending"
                  class="w-full"
                  data-testid="archive-closure-reason"
              /></UFormField>
              <UCheckbox
                v-model="closureConfirmed"
                :label="t('storageGov.archive.disposition.closure.confirm')"
                :disabled="busy || !!pending?.closure || !pending"
                data-testid="archive-closure-confirm"
              />
              <UButton
                :loading="busy"
                :disabled="
                  busy ||
                  !pending ||
                  !!pending.closure ||
                  !requestReview ||
                  !closureConfirmed ||
                  !validClosureReason ||
                  !!contextError ||
                  !closureAvailable
                "
                color="warning"
                class="text-black"
                data-testid="archive-closure-submit"
                @click="closeRequest"
                >{{
                  t("storageGov.archive.disposition.closure.submit")
                }}</UButton
              >
            </div></template
          >
          <template #verification
            ><div class="space-y-4">
              <p>{{ t("storageGov.archive.disposition.verifyHint") }}</p>
              <p v-if="progress" role="status">
                {{ progress.objects }} · {{ bytes(progress.bytes) }}
              </p>
              <div
                v-if="review"
                class="rounded-lg border border-default p-3"
                data-testid="archive-disposition-verified"
              >
                <p>{{ review.object_count }} · {{ bytes(review.bytes) }}</p>
                <p class="mt-1 text-xs text-muted">
                  {{
                    t("storageGov.archive.disposition.policyRevision", {
                      revision: review.policy_revision,
                    })
                  }}
                </p>
              </div>
              <div class="flex flex-wrap gap-2">
                <UButton
                  :loading="busy"
                  :disabled="!!pending || !eligible"
                  data-testid="archive-disposition-verify"
                  @click="verify"
                  >{{
                    t("storageGov.archive.disposition.startReview")
                  }}</UButton
                ><UButton
                  :disabled="!review || busy || !!pending"
                  color="neutral"
                  variant="outline"
                  @click="section = 'confirm'"
                  >{{ t("storageGov.archive.disposition.next") }}</UButton
                >
              </div>
            </div></template
          >
          <template #confirm
            ><div class="space-y-4">
              <UAlert
                color="warning"
                variant="soft"
                :ui="{ title: 'text-highlighted', description: 'text-toned' }"
                :title="t('storageGov.archive.disposition.effect')"
                :description="t('storageGov.archive.disposition.effectHint')"
              /><UFormField :label="t('storageGov.archive.disposition.reason')"
                ><UTextarea
                  v-model="reason"
                  :rows="3"
                  :maxlength="256"
                  :disabled="busy || !!pending || !!result"
                  class="w-full"
                  data-testid="archive-disposition-reason" /></UFormField
              ><UCheckbox
                v-model="confirmed"
                :label="t('storageGov.archive.disposition.confirmLabel')"
                :disabled="busy || !!pending || !!result"
                data-testid="archive-disposition-confirm"
              /><UButton
                :loading="busy"
                :disabled="
                  !review ||
                  !confirmed ||
                  !validReason ||
                  busy ||
                  !!pending ||
                  !!result ||
                  !!contextError
                "
                color="warning"
                class="text-black"
                data-testid="archive-disposition-submit"
                @click="submit"
                >{{ t("storageGov.archive.disposition.submit") }}</UButton
              >
            </div></template
          >
        </BackupSectionTabs>
      </section></template
    >
    <template #footer
      ><UButton
        color="neutral"
        variant="outline"
        :disabled="busy || !!pending"
        @click="opened = false"
        >{{ t("common.close") }}</UButton
      ></template
    >
  </UModal>
</template>
<script setup lang="ts">
import ArchiveCaptureRetentionDetails from "./ArchiveCaptureRetentionDetails.vue";
import { computed, ref, watch, onMounted, onBeforeUnmount } from "vue";
import { useI18n } from "vue-i18n";
import BackupSectionTabs from "./BackupSectionTabs.vue";
import {
  useArchiveOpsService,
  type ArchiveJob,
} from "~/composables/api/services/archiveOpsService";
import { useArchiveDispositionService } from "~/composables/api/services/archiveDispositionService";
import { AUTH_SESSION_EVENT, readAuthSession } from "~/utils/auth-session";
import {
  ARCHIVE_DISPOSITION_EVENT,
  readDispositionIntent,
  saveDispositionIntent,
  settleDisposition,
  dispositionDigest,
  closureDigest,
  saveClosureIntent,
  type InterruptionRequestReview,
  type CloseInterruptionInput,
  type DispositionIntent,
  type InterruptionReview,
  type DispositionReceipt,
  type DispositionInput,
} from "~/utils/archive-disposition";
import { formatStorageBytes as bytes } from "~/utils/storage-capacity";
const props = defineProps<{
    open: boolean;
    jobId?: string;
    connectionStatus?: string;
    revision?: number;
    closureAvailable?: boolean;
  }>(),
  emit = defineEmits<{
    "update:open": [boolean];
    attention: [boolean];
    changed: [];
    busy: [boolean];
  }>();
const { t } = useI18n(),
  api = useArchiveDispositionService(),
  archives = useArchiveOpsService();
const pending = ref<DispositionIntent>(),
  review = ref<InterruptionReview>(),
  result = ref<DispositionReceipt>(),
  job = ref<ArchiveJob>();
const identityVersion = ref(0);
const closureMode = computed(
  () => !!pending.value || result.value?.state === "closed_unknown",
);
const requestReview = ref<InterruptionRequestReview>(),
  closureReason = ref(""),
  closureConfirmed = ref(false);
const validClosureReason = computed(
  () =>
    Array.from(closureReason.value.trim()).length >= 5 &&
    new TextEncoder().encode(closureReason.value.trim()).length <= 1024 &&
    !/[\u0000\r]/.test(closureReason.value),
);
const busy = ref(false),
  loading = ref(false),
  error = ref(""),
  contextError = ref(""),
  reason = ref(""),
  confirmed = ref(false),
  section = ref("overview"),
  progress = ref<{ objects: number; bytes: number }>();
const sections = computed(() =>
  (pending.value
    ? ["overview", "requestReview", "requestClose"]
    : ["overview", "verification", "confirm"]
  ).map((value) => ({
    value,
    label: t("storageGov.archive.disposition.tabs." + value),
  })),
);
const restoredOpen = ref(false);
const effectiveId = computed(
  () => pending.value?.job_id || props.jobId || result.value?.job_id || "",
);
const opened = computed({
  get: () => props.open || restoredOpen.value || !!pending.value,
  set: (v: boolean) => {
    if (!v && (busy.value || pending.value)) return;
    restoredOpen.value = v;
    emit("update:open", v);
  },
});
const validReason = computed(
  () =>
    Array.from(reason.value.trim()).length >= 5 &&
    new TextEncoder().encode(reason.value.trim()).length <= 1024 &&
    !/[\u0000\r]/.test(reason.value),
);
const eligible = computed(
  () =>
    job.value?.status === "interrupted" &&
    !["unconfirmed", "active", "legacy_unproven"].includes(
      job.value.command_drain_state || "",
    ) &&
    job.value.interruption_pending &&
    !job.value.imported &&
    !job.value.recovered &&
    !job.value.migrated_from,
);
let controller: AbortController | undefined,
  loadController: AbortController | undefined,
  generation = 0,
  disposed = false,
  ownerKey = "";
function owner() {
  const s = readAuthSession();
  return s?.status === "active"
    ? { sessionId: s.sessionId, tenantUuid: s.tenantUuid }
    : undefined;
}
function notify() {
  window.dispatchEvent(new Event(ARCHIVE_DISPOSITION_EVENT));
}
function sync() {
  const o = owner(),
    next = JSON.stringify(o);
  if (next !== ownerKey) {
    generation++;
    identityVersion.value = generation;
    job.value = undefined;
    controller?.abort();
    loadController?.abort();
    busy.value = false;
    pending.value = undefined;
    review.value = undefined;
    result.value = undefined;
    reason.value = "";
    confirmed.value = false;
    requestReview.value = undefined;
    closureReason.value = "";
    closureConfirmed.value = false;
    ownerKey = next;
    restoredOpen.value = false;
  }
  contextError.value = "";
  try {
    pending.value = o ? readDispositionIntent(sessionStorage, o) : undefined;
    if (pending.value) restoredOpen.value = true;
  } catch {
    contextError.value = t("storageGov.archive.runRequest.storageProblem");
    restoredOpen.value = true;
  }
  if (!o)
    contextError.value = t("storageGov.archive.runRequest.sessionRequired");
}
watch(pending, (v, old) => {
  if (v && !old) section.value = "overview";
});
watch([pending, contextError], () =>
  emit("attention", !!pending.value || !!contextError.value),
);
watch(busy, (v) => emit("busy", v));
watch(
  [opened, effectiveId, identityVersion, () => props.revision],
  async ([open, id], old) => {
    if (!open || !id) return;
    if (old && props.revision !== old[3]) {
      requestReview.value = undefined;
      closureConfirmed.value = false;
    }
    if (old && props.revision !== old[3] && !pending.value) {
      review.value = undefined;
      confirmed.value = false;
    }
    if (id !== old?.[1]) {
      review.value = undefined;
      result.value = undefined;
      section.value = "overview";
      reason.value = "";
      confirmed.value = false;
    }
    loadController?.abort();
    const c = (loadController = new AbortController()),
      g = generation;
    loading.value = true;
    try {
      const out = await archives.job(id, c.signal);
      if (!disposed && !c.signal.aborted && g === generation) job.value = out;
    } catch (e) {
      if (!disposed && !c.signal.aborted && g === generation)
        error.value = message(e);
    } finally {
      if (g === generation) loading.value = false;
    }
  },
  { immediate: true },
);
function message(e: any) {
  const value = e?.data?.message || e?.message || String(e);
  const keys: Record<string, string> = {
    ARCHIVE_DISPOSITION_UNCONFIRMED: "resultUnknown",
    ARCHIVE_DISPOSITION_RECEIPT_MISMATCH: "receiptProblem",
    ARCHIVE_DISPOSITION_RESPONSE_INVALID: "receiptProblem",
    ARCHIVE_DISPOSITION_CONTEXT_INVALID: "contextProblem",
    ARCHIVE_REVIEW_INCOMPLETE: "reviewIncomplete",
    ARCHIVE_PAUSE_REQUIRED: "pauseRequired",
    ARCHIVE_CLOSURE_UNCONFIRMED: "resultUnknown",
    ARCHIVE_REVIEW_STALE: "reviewIncomplete",
  };
  return keys[value]
    ? t("storageGov.archive.disposition." + keys[value])
    : value;
}
async function perform(fn: (signal: AbortSignal, g: number) => Promise<void>) {
  if (busy.value) return;
  const c = (controller = new AbortController()),
    g = generation;
  busy.value = true;
  error.value = "";
  try {
    await fn(c.signal, g);
  } catch (e) {
    if (!disposed && !c.signal.aborted && g === generation)
      error.value = message(e);
  } finally {
    if (!disposed && g === generation) busy.value = false;
  }
}
async function verify() {
  if (pending.value) return;
  result.value = undefined;
  review.value = undefined;
  progress.value = undefined;
  await perform(async (signal, g) => {
    const revision = props.revision;
    const v = await api.review(effectiveId.value, signal, (p) => {
      if (g === generation && !disposed) progress.value = p;
    });
    if (g === generation && !disposed) {
      if (revision !== props.revision)
        throw new Error("ARCHIVE_REVIEW_INCOMPLETE");
      review.value = v;
    }
  });
}
function accept(intent: DispositionIntent, v: DispositionReceipt) {
  const o = owner();
  if (disposed || !o || JSON.stringify(o) !== JSON.stringify(intent.owner))
    return;
  settleDisposition(sessionStorage, intent, v);
  result.value = v;
  pending.value = undefined;
  requestReview.value = undefined;
  section.value = "overview";
  review.value = undefined;
  notify();
  emit("changed");
}
async function submit() {
  if (
    !review.value ||
    pending.value ||
    !confirmed.value ||
    !validReason.value ||
    contextError.value
  )
    return;
  await perform(async (signal, g) => {
    const o = owner();
    if (!o) throw new Error("ARCHIVE_DISPOSITION_SESSION_REQUIRED");
    const jobId = effectiveId.value,
      input: DispositionInput = {
        request_id: crypto.randomUUID(),
        expected_version: review.value!.review_version,
        action: "retain_and_close",
        reason: reason.value.trim(),
        confirm: true,
      };
    const digest = await dispositionDigest(jobId, input);
    if (g !== generation || disposed) return;
    const intent = saveDispositionIntent(sessionStorage, {
      version: 1,
      owner: o,
      request_id: input.request_id,
      job_id: jobId,
      expected_version: input.expected_version,
      input_digest: digest,
      created_at: new Date().toISOString(),
    });
    pending.value = intent;
    notify();
    const receipt = await api.retain(jobId, input, signal);
    if (g === generation) accept(intent, receipt);
  });
}
async function inspectRequest() {
  const intent = pending.value;
  if (!intent || intent.closure || !props.closureAvailable) return;
  requestReview.value = undefined;
  closureConfirmed.value = false;
  await perform(async (signal, g) => {
    const revision = props.revision;
    const v = await api.requestReview(intent.request_id, signal);
    if (g !== generation || disposed) return;
    if (
      revision !== props.revision ||
      v.request_id !== intent.request_id ||
      v.job_id !== intent.job_id ||
      v.input_digest !== intent.input_digest
    )
      throw new Error("ARCHIVE_DISPOSITION_RECEIPT_MISMATCH");
    requestReview.value = v;
  });
}
async function closeRequest() {
  const intent = pending.value,
    proof = requestReview.value;
  if (
    !intent ||
    intent.closure ||
    !proof ||
    !closureConfirmed.value ||
    !validClosureReason.value ||
    !props.closureAvailable ||
    contextError.value
  )
    return;
  await perform(async (signal, g) => {
    const input: CloseInterruptionInput = {
      request_id: crypto.randomUUID(),
      expected_version: proof.review_version,
      action: "close_unknown_keep_protection",
      reason: closureReason.value.trim(),
      confirm: true,
    };
    const digest = await closureDigest(
      intent.request_id,
      intent.input_digest,
      input,
    );
    if (g !== generation || disposed) return;
    const saved = saveClosureIntent(sessionStorage, intent, {
      request_id: input.request_id,
      expected_version: input.expected_version,
      input_digest: digest,
    });
    pending.value = saved;
    notify();
    const receipt = await api.closeRequest(intent.request_id, input, signal);
    if (g === generation) accept(saved, receipt);
  });
}
async function reconcile() {
  const intent = pending.value;
  if (!intent) return;
  await perform(async (signal, g) => {
    const receipt = await api.request(intent.request_id, signal);
    if (g === generation) accept(intent, receipt);
  });
}
onMounted(() => {
  sync();
  window.addEventListener(AUTH_SESSION_EVENT, sync);
  window.addEventListener(ARCHIVE_DISPOSITION_EVENT, sync);
  window.addEventListener("storage", sync);
});
onBeforeUnmount(() => {
  disposed = true;
  generation++;
  controller?.abort();
  loadController?.abort();
  emit("busy", false);
  emit("attention", false);
  window.removeEventListener(AUTH_SESSION_EVENT, sync);
  window.removeEventListener(ARCHIVE_DISPOSITION_EVENT, sync);
  window.removeEventListener("storage", sync);
});
</script>
