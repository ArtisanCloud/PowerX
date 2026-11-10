<template>
  <UModal
    v-model:open="opened"
    :dismissible="!busy && !pending"
    :title="t('storageGov.archive.commandConfirmation.title')"
    :description="t('storageGov.archive.commandConfirmation.hint')"
    :ui="{ content: 'max-w-3xl' }"
  >
    <template #body
      ><section
        class="min-w-0 space-y-4"
        data-testid="archive-command-confirmation-dialog"
      >
        <UAlert
          v-if="connectionStatus === 'disconnected'"
          color="warning"
          variant="soft"
          :ui="alertUI"
          :title="t('storageGov.archive.disconnected')"
          data-testid="command-confirmation-disconnected"
        />
        <UAlert
          v-if="contextError"
          color="error"
          :title="t('storageGov.archive.runRequest.contextUnavailable')"
          :description="contextError"
          data-testid="command-confirmation-context"
        />
        <UAlert
          v-if="error"
          color="error"
          :title="t('storageGov.requestFailed')"
          :description="error"
          data-testid="command-confirmation-error"
        />
        <UAlert
          v-if="pending"
          color="warning"
          variant="soft"
          :ui="alertUI"
          :title="t('storageGov.archive.commandConfirmation.pending')"
          :description="t('storageGov.archive.commandConfirmation.pendingHint')"
          data-testid="command-confirmation-pending"
          ><template #actions
            ><div class="space-y-2">
              <p class="break-all text-xs text-toned">
                {{ pending.request_id }}
              </p>
              <UButton
                :disabled="busy"
                color="neutral"
                variant="outline"
                data-testid="command-confirmation-reconcile"
                @click="reconcile"
                >{{ t("storageGov.archive.runRequest.reconcile") }}</UButton
              >
            </div></template
          ></UAlert
        >
        <UAlert
          v-if="result"
          :color="result.state === 'confirmed' ? 'success' : 'warning'"
          :title="
            t(
              result.state === 'confirmed'
                ? 'storageGov.archive.commandConfirmation.success'
                : 'storageGov.archive.commandConfirmation.rejected',
            )
          "
          :description="
            t(
              result?.target_id || review?.target_id
                ? 'storageGov.archive.commandConfirmation.probe.effect'
                : 'storageGov.archive.commandConfirmation.effectHint',
            )
          "
          data-testid="command-confirmation-result"
        />
        <BackupSectionTabs
          v-model="section"
          :items="sections"
          :label="t('storageGov.archive.commandConfirmation.title')"
        >
          <template #overview
            ><div class="space-y-4">
              <p>
                {{
                  t(
                    review?.subject === "probe" || result?.target_id
                      ? "storageGov.archive.commandConfirmation.probe.effect"
                      : "storageGov.archive.commandConfirmation.scope",
                  )
                }}
              </p>
              <p class="break-all text-xs text-muted">{{ effectiveId }}</p>
              <div
                v-if="result"
                class="rounded-lg border border-default p-3 text-sm space-y-2"
                data-testid="command-confirmation-audit"
              >
                <p class="break-words whitespace-pre-wrap">
                  {{ result.reason }}
                </p>
                <p>
                  {{
                    t("storageGov.archive.disposition.closure.actor", {
                      id: result.operator,
                    })
                  }}
                  · {{ new Date(result.at).toLocaleString() }}
                </p>
                <p class="break-all text-xs text-muted">
                  {{ result.request_id }}
                </p>
              </div>
              <UButton
                :disabled="busy || !!pending || !!contextError"
                @click="section = 'review'"
                >{{
                  t("storageGov.archive.commandConfirmation.startReview")
                }}</UButton
              >
            </div></template
          >
          <template #review
            ><div class="space-y-4">
              <p>
                {{ t("storageGov.archive.commandConfirmation.reviewHint") }}
              </p>
              <div
                v-if="review"
                class="rounded-lg border border-default p-3 space-y-3 text-sm"
                data-testid="command-confirmation-reviewed"
              >
                <p :class="review.eligible ? 'text-success' : 'text-warning'">
                  {{
                    t(
                      "storageGov.archive.commandConfirmation.problems." +
                        (review.eligible
                          ? "eligible"
                          : review.problem || "unknown"),
                    )
                  }}
                </p>
                <div
                  v-if="review.probe"
                  class="rounded-lg bg-elevated p-3 space-y-2"
                  data-testid="command-probe-scope"
                >
                  <h4 class="font-medium">
                    {{
                      t(
                        "storageGov.archive.commandConfirmation.probe.originalTarget",
                      )
                    }}
                    · {{ review.probe.name }}
                  </h4>
                  <p class="break-all text-xs text-muted">
                    {{ review.probe.target_id }}
                  </p>
                  <p>
                    {{
                      t(
                        "storageGov.archive.commandConfirmation.probe." +
                          review.probe.target_state,
                      )
                    }}
                    · {{ review.probe.original_revision }} →
                    {{ review.probe.current_revision || "—" }}
                  </p>
                  <p class="text-warning">
                    {{
                      t("storageGov.archive.commandConfirmation.probe.effect")
                    }}
                  </p>
                  <details>
                    <summary>
                      {{
                        t(
                          "storageGov.archive.commandConfirmation.probe.references",
                          { count: review.probe.policies.length },
                        )
                      }}
                    </summary>
                    <ul class="mt-2 space-y-2">
                      <li
                        v-for="p in review.probe.policies"
                        :key="p.id"
                        class="break-words"
                      >
                        {{ p.name }} ·
                        {{
                          p.paused
                            ? t(
                                "storageGov.archive.commandConfirmation.probe.paused",
                              )
                            : t(
                                "storageGov.archive.commandConfirmation.probe.active",
                              )
                        }}
                        <p class="text-xs text-muted break-all">{{ p.id }}</p>
                      </li>
                    </ul>
                  </details>
                </div>
                <dl class="grid gap-3 sm:grid-cols-2">
                  <div>
                    <dt class="text-muted">
                      {{ t("storageGov.archive.commandConfirmation.kind") }}
                    </dt>
                    <dd>{{ review.kind || "—" }}</dd>
                  </div>
                  <div>
                    <dt class="text-muted">
                      {{ t("storageGov.archive.commandConfirmation.boot") }}
                    </dt>
                    <dd>
                      {{
                        t(
                          review.boot_matches
                            ? "storageGov.archive.commandConfirmation.matched"
                            : "storageGov.archive.commandConfirmation.unconfirmed",
                        )
                      }}
                    </dd>
                  </div>
                </dl>
                <details
                  v-if="review.identity"
                  class="rounded-lg border border-default p-3"
                >
                  <summary>
                    {{
                      t(
                        "storageGov.archive.commandConfirmation.identityDetails",
                      )
                    }}
                  </summary>
                  <dl class="mt-3 grid gap-3 text-xs sm:grid-cols-2">
                    <div>
                      <dt>PID / PGID</dt>
                      <dd>
                        {{ review.identity.pid }} / {{ review.identity.pgid }}
                      </dd>
                    </div>
                    <div>
                      <dt>UID</dt>
                      <dd>{{ review.identity.uid }}</dd>
                    </div>
                    <div>
                      <dt>
                        {{
                          t("storageGov.archive.commandConfirmation.started")
                        }}
                      </dt>
                      <dd>{{ review.identity.started }}</dd>
                    </div>
                    <div>
                      <dt>
                        {{
                          t("storageGov.archive.commandConfirmation.commandId")
                        }}
                      </dt>
                      <dd class="break-all">{{ review.command_id }}</dd>
                    </div>
                  </dl>
                </details>
              </div>
              <div class="flex flex-wrap gap-2">
                <UButton
                  :loading="busy"
                  :disabled="!!pending || !!contextError"
                  data-testid="command-confirmation-review"
                  @click="inspect"
                  >{{
                    t("storageGov.archive.commandConfirmation.startReview")
                  }}</UButton
                ><UButton
                  color="neutral"
                  variant="outline"
                  :disabled="busy || !review?.eligible || !!pending"
                  @click="section = 'confirm'"
                  >{{ t("storageGov.archive.disposition.next") }}</UButton
                >
              </div>
            </div></template
          >
          <template #confirm
            ><div class="space-y-4">
              <div
                v-if="review?.probe"
                class="rounded-lg border border-default p-3 text-sm"
                data-testid="command-probe-confirm-scope"
              >
                <p class="font-medium">
                  {{
                    t(
                      "storageGov.archive.commandConfirmation.probe.originalTarget",
                    )
                  }}
                  · {{ review.probe.name }}
                </p>
                <p class="text-xs text-muted break-all">
                  {{ review.probe.target_id }}
                </p>
                <p class="mt-1 text-warning">
                  {{
                    t(
                      "storageGov.archive.commandConfirmation.probe." +
                        review.probe.target_state,
                    )
                  }}
                </p>
              </div>
              <UAlert
                color="warning"
                variant="soft"
                :ui="alertUI"
                :title="t('storageGov.archive.commandConfirmation.effect')"
                :description="
                  t(
                    result?.target_id || review?.target_id
                      ? 'storageGov.archive.commandConfirmation.probe.effect'
                      : 'storageGov.archive.commandConfirmation.effectHint',
                  )
                "
              /><UFormField :label="t('storageGov.archive.disposition.reason')"
                ><UTextarea
                  v-model="reason"
                  :rows="3"
                  :maxlength="256"
                  :disabled="busy || !!pending || !!result"
                  class="w-full"
                  data-testid="command-confirmation-reason" /></UFormField
              ><UCheckbox
                v-model="confirmed"
                :disabled="busy || !!pending || !!result"
                :label="
                  t('storageGov.archive.commandConfirmation.confirmLabel')
                "
                data-testid="command-confirmation-approve"
              /><UButton
                :loading="busy"
                :disabled="
                  busy ||
                  !!pending ||
                  !!result ||
                  !!contextError ||
                  !review?.eligible ||
                  !validReason ||
                  !confirmed
                "
                color="warning"
                class="text-black"
                data-testid="command-confirmation-submit"
                @click="submit"
                >{{
                  t("storageGov.archive.commandConfirmation.submit")
                }}</UButton
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
import { computed, ref, watch, onMounted, onBeforeUnmount } from "vue";
import { useI18n } from "vue-i18n";
import BackupSectionTabs from "./BackupSectionTabs.vue";
import { useArchiveCommandConfirmationService } from "~/composables/api/services/archiveCommandConfirmationService";
import { AUTH_SESSION_EVENT, readAuthSession } from "~/utils/auth-session";
import {
  ARCHIVE_COMMAND_CONFIRMATION_EVENT,
  readCommandConfirmationIntent,
  saveCommandConfirmationIntent,
  settleCommandConfirmation,
  commandConfirmationDigest,
  type CommandDrainReview,
  type CommandConfirmationReceipt,
  type CommandConfirmationIntent,
  type CommandConfirmationInput,
} from "~/utils/archive-command-confirmation";
const props = defineProps<{
    open: boolean;
    recordId?: string;
    revision?: number;
    connectionStatus?: string;
  }>(),
  emit = defineEmits<{
    "update:open": [boolean];
    attention: [boolean];
    busy: [boolean];
    changed: [];
  }>();
const { t } = useI18n(),
  api = useArchiveCommandConfirmationService(),
  alertUI = { title: "text-highlighted", description: "text-toned" };
const pending = ref<CommandConfirmationIntent>(),
  review = ref<CommandDrainReview>(),
  result = ref<CommandConfirmationReceipt>(),
  busy = ref(false),
  error = ref(""),
  contextError = ref(""),
  reason = ref(""),
  confirmed = ref(false),
  section = ref("overview"),
  restoredOpen = ref(false);
const effectiveId = computed(
  () =>
    pending.value?.record_id || props.recordId || result.value?.record_id || "",
);
const opened = computed({
  get: () => props.open || restoredOpen.value || !!pending.value,
  set: (v: boolean) => {
    if (!v && (busy.value || pending.value)) return;
    restoredOpen.value = v;
    emit("update:open", v);
  },
});
const sections = computed(() =>
  ["overview", "review", "confirm"].map((value) => ({
    value,
    label: t("storageGov.archive.commandConfirmation.tabs." + value),
  })),
);
const validReason = computed(
  () =>
    Array.from(reason.value.trim()).length >= 5 &&
    new TextEncoder().encode(reason.value.trim()).length <= 1024 &&
    !/[\u0000\r]/.test(reason.value),
);
let generation = 0,
  disposed = false,
  ownerKey = "",
  controller: AbortController | undefined;
function owner() {
  const v = readAuthSession();
  return v?.status === "active"
    ? { sessionId: v.sessionId, tenantUuid: v.tenantUuid }
    : undefined;
}
function notify() {
  window.dispatchEvent(new Event(ARCHIVE_COMMAND_CONFIRMATION_EVENT));
}
function sync() {
  const o = owner(),
    key = JSON.stringify(o);
  if (key !== ownerKey) {
    generation++;
    controller?.abort();
    ownerKey = key;
    busy.value = false;
    pending.value = undefined;
    review.value = undefined;
    result.value = undefined;
    reason.value = "";
    confirmed.value = false;
    restoredOpen.value = false;
    error.value = "";
  }
  contextError.value = "";
  try {
    pending.value = o
      ? readCommandConfirmationIntent(sessionStorage, o)
      : undefined;
    if (pending.value) restoredOpen.value = true;
  } catch {
    contextError.value = t("storageGov.archive.runRequest.storageProblem");
    restoredOpen.value = true;
  }
  if (!o)
    contextError.value = t("storageGov.archive.runRequest.sessionRequired");
}
watch([pending, contextError], () =>
  emit("attention", !!pending.value || !!contextError.value),
);
watch(busy, (v) => emit("busy", v));
watch(
  () => props.revision,
  () => {
    review.value = undefined;
    confirmed.value = false;
  },
);
watch(effectiveId, (v, old) => {
  if (v !== old) {
    review.value = undefined;
    result.value = undefined;
    reason.value = "";
    confirmed.value = false;
    section.value = "overview";
  }
});
function message(e: any) {
  const code = e?.data?.message || e?.message || String(e);
  return [
    "ARCHIVE_COMMAND_CONFIRMATION_UNCONFIRMED",
    "ARCHIVE_COMMAND_CONFIRMATION_RESPONSE_INVALID",
  ].includes(code)
    ? t("storageGov.archive.commandConfirmation.pendingHint")
    : code;
}
async function perform(fn: (s: AbortSignal, g: number) => Promise<void>) {
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
async function inspect() {
  if (pending.value || contextError.value) return;
  review.value = undefined;
  result.value = undefined;
  confirmed.value = false;
  await perform(async (signal, g) => {
    const version = props.revision;
    const v = await api.review(effectiveId.value, signal);
    if (g !== generation || disposed) return;
    if (version !== props.revision || v.record_id !== effectiveId.value)
      throw new Error("ARCHIVE_REVIEW_STALE");
    review.value = v;
  });
}
function accept(i: CommandConfirmationIntent, v: CommandConfirmationReceipt) {
  const o = owner();
  if (disposed || !o || JSON.stringify(o) !== JSON.stringify(i.owner)) return;
  settleCommandConfirmation(sessionStorage, i, v);
  pending.value = undefined;
  result.value = v;
  review.value = undefined;
  section.value = "overview";
  notify();
  emit("changed");
}
async function submit() {
  const proof = review.value;
  if (
    !proof?.eligible ||
    pending.value ||
    !confirmed.value ||
    !validReason.value ||
    contextError.value
  )
    return;
  await perform(async (signal, g) => {
    const o = owner();
    if (!o) throw new Error("ARCHIVE_COMMAND_CONFIRMATION_SESSION_REQUIRED");
    const input: CommandConfirmationInput = {
      request_id: crypto.randomUUID(),
      expected_version: proof.review_version,
      execution_id: proof.execution_id,
      command_id: proof.command_id,
      action: "confirm_local_command_drain",
      reason: reason.value.trim(),
      confirm: true,
      ...(proof.target_id ? { target_id: proof.target_id } : {}),
    };
    const digest = await commandConfirmationDigest(proof.record_id, input);
    if (g !== generation || disposed) return;
    const i = saveCommandConfirmationIntent(sessionStorage, {
      version: 1,
      owner: o,
      request_id: input.request_id,
      record_id: proof.record_id,
      job_id: proof.job_id,
      ...(proof.target_id ? { target_id: proof.target_id } : {}),
      execution_id: proof.execution_id,
      command_id: proof.command_id,
      expected_version: proof.review_version,
      input_digest: digest,
      created_at: new Date().toISOString(),
    });
    pending.value = i;
    notify();
    const v = await api.confirm(proof.record_id, input, signal);
    if (g === generation) accept(i, v);
  });
}
async function reconcile() {
  const i = pending.value;
  if (!i) return;
  await perform(async (signal, g) => {
    const v = await api.request(i.request_id, signal);
    if (g === generation) accept(i, v);
  });
}
onMounted(() => {
  sync();
  window.addEventListener(AUTH_SESSION_EVENT, sync);
  window.addEventListener(ARCHIVE_COMMAND_CONFIRMATION_EVENT, sync);
  window.addEventListener("storage", sync);
});
onBeforeUnmount(() => {
  disposed = true;
  generation++;
  controller?.abort();
  emit("busy", false);
  emit("attention", false);
  window.removeEventListener(AUTH_SESSION_EVENT, sync);
  window.removeEventListener(ARCHIVE_COMMAND_CONFIRMATION_EVENT, sync);
  window.removeEventListener("storage", sync);
});
</script>
