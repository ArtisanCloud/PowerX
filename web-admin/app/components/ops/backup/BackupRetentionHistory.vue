<template>
  <section class="min-w-0 space-y-4" data-testid="retention-history">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold">{{ t("backupHistory.title") }}</h2>
        <p class="mt-1 text-sm text-muted">{{ t("backupHistory.hint") }}</p>
      </div>
      <UButton
        color="neutral"
        variant="outline"
        :loading="loading"
        @click="refresh"
        >{{ t("backupHistory.refresh") }}</UButton
      >
    </header>
    <UAlert v-if="error" color="warning" :title="error" />
    <UAlert v-if="sourceError" color="warning" :title="sourceError" />
    <UAlert v-if="historyError" color="warning" :title="historyError" />
    <UAlert
      v-if="events.status.value === 'disconnected'"
      color="warning"
      :title="t('backupHistory.disconnected')"
    />
    <UAlert
      v-if="preview && !preview.pause_ready"
      color="warning"
      :title="t('backupHistory.pauseRequired')"
      ><template #actions
        ><UButton to="/monitor/storage" color="neutral" variant="outline">{{
          t("backupHistory.safety")
        }}</UButton></template
      ></UAlert
    >
    <UAlert
      v-if="preview?.foreign"
      color="warning"
      :title="t('backupHistory.foreign', { count: preview.foreign })"
    />
    <div v-if="preview" class="grid gap-3 sm:grid-cols-3">
      <div class="rounded-xl border border-default p-4">
        <p class="text-sm text-muted">{{ t("backupHistory.scope") }}</p>
        <p class="mt-2 text-xl font-semibold tabular-nums">
          {{ preview.covered }} / {{ preview.total }}
        </p>
      </div>
      <div class="rounded-xl border border-default p-4">
        <p class="text-sm text-muted">{{ t("backupHistory.gaps") }}</p>
        <p class="mt-2 text-xl font-semibold tabular-nums">
          {{ preview.missing }}
        </p>
      </div>
      <div class="rounded-xl border border-default p-4">
        <p class="text-sm text-muted">
          {{ t("backupHistory.unknownResults") }}
        </p>
        <p class="mt-2 text-xl font-semibold tabular-nums">
          {{ preview.unknown }}
        </p>
      </div>
    </div>
    <BackupSectionTabs
      v-model="section"
      :items="sections"
      :label="t('backupHistory.title')"
    >
      <template #inventory>
        <div class="space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <UBadge :color="preview?.complete ? 'success' : 'warning'">{{
              t(
                preview?.complete
                  ? "backupHistory.covered"
                  : "backupHistory.reviewNeeded",
              )
            }}</UBadge
            ><select
              v-model="filter"
              class="min-h-10 rounded-lg border border-default bg-default px-3 text-sm"
              data-testid="history-filter"
              @change="
                page = 1;
                loadPreview();
              "
            >
              <option v-for="key in filters" :key="key" :value="key">
                {{ t("backupHistory.filters." + key) }}
              </option>
            </select>
          </div>
          <p class="text-sm text-muted">
            {{ t("backupHistory.snapshotOnly") }}
          </p>
          <UAlert
            v-if="preview?.problem"
            color="warning"
            :title="t('backupHistory.blocked')"
            :description="preview.problem"
          />
          <div
            v-if="preview"
            class="overflow-x-auto rounded-xl border border-default"
          >
            <table class="w-full text-left text-sm">
              <thead class="bg-muted text-muted">
                <tr>
                  <th class="p-3">{{ t("backupHistory.request") }}</th>
                  <th class="p-3">{{ t("backupHistory.originalActor") }}</th>
                  <th class="p-3">{{ t("backupHistory.result") }}</th>
                  <th class="p-3">{{ t("backupHistory.coverage") }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-default">
                <tr v-for="row in preview.items" :key="row.row_id">
                  <td class="p-3">
                    <span class="break-all font-mono text-xs">{{
                      row.request_id
                    }}</span>
                    <p class="mt-1 text-xs text-muted">
                      #{{ row.artifact_id }} · {{ time(row.created_at) }}
                    </p>
                  </td>
                  <td class="p-3 tabular-nums">{{ row.actor_id }}</td>
                  <td class="p-3 whitespace-nowrap">
                    {{
                      t(
                        "backupHistory.results." +
                          (["checking", "retained", "rejected"].includes(
                            row.state,
                          )
                            ? row.state
                            : "invalid"),
                      )
                    }}
                  </td>
                  <td class="p-3">
                    <UBadge
                      :color="
                        row.coverage === 'covered' ? 'success' : 'warning'
                      "
                      >{{ t("backupHistory.filters." + row.coverage) }}</UBadge
                    >
                    <p
                      v-if="row.problem"
                      class="mt-1 max-w-48 break-all text-xs text-muted"
                    >
                      {{ row.problem }}
                    </p>
                  </td>
                </tr>
              </tbody>
            </table>
            <p v-if="!preview.items.length" class="p-6 text-center text-muted">
              {{ t("backupHistory.empty") }}
            </p>
          </div>
          <footer
            class="flex flex-wrap items-center justify-between gap-2 text-sm"
          >
            <span>{{ page }} · {{ preview?.filtered || 0 }}</span>
            <div class="flex gap-2">
              <UButton
                color="neutral"
                variant="outline"
                :disabled="loading || page <= 1"
                @click="
                  page--;
                  loadPreview();
                "
                >{{ t("backupCenter.previous") }}</UButton
              ><UButton
                color="neutral"
                variant="outline"
                :disabled="loading || page * 20 >= (preview?.filtered || 0)"
                @click="
                  page++;
                  loadPreview();
                "
                >{{ t("backupCenter.next") }}</UButton
              ><UButton data-testid="history-open-import" @click="openImport">{{
                t("backupHistory.configureImport")
              }}</UButton>
            </div>
          </footer>
        </div>
      </template>
      <template #import>
        <div class="space-y-4" data-testid="history-import">
          <p class="text-sm text-muted">{{ t("backupHistory.importHint") }}</p>
          <UAlert
            v-if="unknown"
            color="warning"
            :title="t('backupHistory.unknown')"
          />
          <div
            v-if="pending"
            class="space-y-3 rounded-xl border border-default p-4"
          >
            <p
              class="break-all font-mono text-xs"
              data-testid="history-frozen-id"
            >
              {{ pending.request_id }}
            </p>
            <p class="text-sm text-muted">{{ t("backupHistory.frozen") }}</p>
            <div class="flex flex-wrap gap-2">
              <UButton
                color="neutral"
                variant="outline"
                :disabled="running || reconciling"
                data-testid="history-reconcile"
                @click="reconcile"
                >{{ t("backupHistory.reconcile") }}</UButton
              ><UButton
                v-if="unknown && (!receipt || receipt.can_resume)"
                color="neutral"
                variant="outline"
                data-testid="history-resume"
                :disabled="running || reconciling"
                @click="submit(pending)"
                >{{ t("backupHistory.resume") }}</UButton
              ><UButton
                v-if="running"
                color="neutral"
                variant="outline"
                @click="stop"
                >{{ t("backupHistory.stop") }}</UButton
              >
            </div>
          </div>
          <template v-else-if="!receipt || receipt.state === 'pending'"
            ><UFormField :label="t('backupHistory.reason')"
              ><UTextarea
                v-model="reason"
                :rows="3"
                :maxlength="512"
                class="w-full"
                data-testid="history-reason" /></UFormField
            ><UCheckbox
              v-model="confirm"
              :label="t('backupHistory.confirm')"
              data-testid="history-confirm"
            />
            <details class="text-xs text-muted">
              <summary>{{ t("backupHistory.details") }}</summary>
              <p class="mt-2 break-all font-mono">{{ preview?.snapshot }}</p>
            </details>
            <UButton
              data-testid="history-submit"
              :disabled="
                !preview?.can_import ||
                !sourceFresh ||
                loading ||
                !confirm ||
                [...reason.trim()].length < 3 ||
                [...reason.trim()].length > 256
              "
              @click="begin"
              >{{ t("backupHistory.submit") }}</UButton
            ></template
          >
          <div
            v-if="receipt"
            class="space-y-2 rounded-xl border border-default p-4"
            data-testid="history-receipt"
          >
            <UBadge
              :color="receipt.state === 'completed' ? 'success' : 'warning'"
              >{{ t("backupHistory.states." + receipt.state) }}</UBadge
            >
            <p class="break-all font-mono text-xs">{{ receipt.request_id }}</p>
            <p class="text-sm">
              {{
                t("backupHistory.proof", {
                  covered: receipt.covered,
                  total: receipt.total,
                  unknown: receipt.unknown,
                })
              }}
            </p>
            <p class="text-sm text-muted">{{ t("backupHistory.proofHint") }}</p>
            <details v-if="receipt.problem" class="text-xs text-muted">
              <summary>{{ t("backupHistory.details") }}</summary>
              <p class="mt-2 break-all">{{ receipt.problem }}</p>
            </details>
            <UButton
              v-if="receipt.state !== 'pending' && !pending"
              color="neutral"
              variant="outline"
              data-testid="history-new-import"
              @click="openImport"
              >{{ t("backupHistory.newImport") }}</UButton
            >
          </div>
        </div>
      </template>
      <template #receipts
        ><div class="space-y-3">
          <p class="text-sm text-muted">
            {{ t("backupHistory.receiptsHint") }}
          </p>
          <p v-if="!receipts.length" class="p-6 text-center text-muted">
            {{ t("backupHistory.empty") }}
          </p>
          <button
            v-for="r in receipts"
            :key="r.request_id"
            type="button"
            class="block w-full rounded-xl border border-default p-4 text-left hover:bg-muted"
            :data-history-receipt="r.request_id"
            @click="choose(r)"
          >
            <UBadge :color="r.state === 'completed' ? 'success' : 'warning'">{{
              t("backupHistory.states." + r.state)
            }}</UBadge>
            <p class="mt-2 break-all font-mono text-xs">{{ r.request_id }}</p>
            <p class="mt-1 text-sm text-muted">
              {{ time(r.completed_at || r.created_at) }} · {{ r.covered }} /
              {{ r.total }}
            </p>
          </button>
        </div></template
      >
    </BackupSectionTabs>
  </section>
</template>
<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from "vue";
import { useI18n } from "vue-i18n";
import BackupSectionTabs from "./BackupSectionTabs.vue";
import {
  useBackupHistoryService,
  type HistoryPreview,
  type HistoryReceipt,
  type HistoryInput,
} from "~/composables/api/services/backupHistoryService";
import { useBackupEvents } from "~/composables/useBackupEvents";
import { AUTH_SESSION_EVENT, readAuthSession } from "~/utils/auth-session";
const { t } = useI18n(),
  api = useBackupHistoryService();
const sourceError = ref(""),
  historyError = ref(""),
  sourceFresh = ref(false);
const section = ref("inventory"),
  preview = ref<HistoryPreview>(),
  receipt = ref<HistoryReceipt>(),
  receipts = ref<HistoryReceipt[]>([]),
  pending = ref<HistoryInput>(),
  reason = ref(""),
  confirm = ref(false),
  unknown = ref(false),
  running = ref(false),
  loading = ref(false),
  reconciling = ref(false),
  error = ref(""),
  page = ref(1),
  filter = ref("all");
const filters = ["all", "missing", "covered", "conflict", "invalid"];
const sections = computed(() =>
  ["inventory", "import", "receipts"].map((value) => ({
    value,
    label: t("backupHistory.sections." + value),
  })),
);
let disposed = false,
  controller: AbortController | undefined,
  previewController: AbortController | undefined,
  recentController: AbortController | undefined,
  readController: AbortController | undefined;
const identity = () => {
  const s = readAuthSession();
  return `${s?.sessionId || ""}:${s?.tenantUuid || ""}:${s?.status || ""}`;
};
let owner = identity();
const time = (v: string) => new Date(v).toLocaleString();
function accept(r: HistoryReceipt) {
  receipt.value = r;
  if (r.state !== "pending") {
    pending.value = undefined;
    unknown.value = false;
    running.value = false;
    confirm.value = false;
    controller?.abort();
  } else if (r.can_resume && !pending.value) {
    pending.value = {
      request_id: r.request_id,
      expected_snapshot: r.expected_snapshot,
      reason: r.reason,
      confirm: true,
    };
    unknown.value = true;
  }
}
async function loadPreview() {
  previewController?.abort();
  const c = (previewController = new AbortController());
  loading.value = true;
  try {
    const v = await api.preview(page.value, filter.value, c.signal);
    if (!disposed && !c.signal.aborted) {
      preview.value = v;
      sourceError.value = "";
      sourceFresh.value = true;
    }
  } catch {
    if (!disposed && !c.signal.aborted) {
      sourceError.value = t("backupHistory.loadFailed");
      sourceFresh.value = false;
    }
  } finally {
    if (!disposed && !c.signal.aborted) loading.value = false;
  }
}
async function loadRecent() {
  recentController?.abort();
  const c = (recentController = new AbortController());
  try {
    const rows = await api.recent(c.signal);
    if (disposed || c.signal.aborted) return;
    receipts.value = rows;
    historyError.value = "";
    if (pending.value) {
      const r = rows.find((r) => r.request_id === pending.value!.request_id);
      if (r) accept(r);
    } else if (!receipt.value) {
      const r = rows.find((r) => r.state === "pending" && r.can_resume);
      if (r) accept(r);
    }
  } catch {
    if (!disposed && !c.signal.aborted)
      historyError.value = t("backupHistory.historyFailed");
  }
}
async function refresh() {
  await Promise.all([loadPreview(), loadRecent()]);
}
function begin() {
  if (
    !preview.value?.can_import ||
    !sourceFresh.value ||
    !confirm.value ||
    pending.value
  )
    return;
  void submit({
    request_id: crypto.randomUUID(),
    expected_snapshot: preview.value.snapshot,
    reason: reason.value.trim(),
    confirm: true,
  });
}
function openImport() {
  section.value = "import";
  if (!pending.value && receipt.value && receipt.value.state !== "pending") {
    receipt.value = undefined;
    reason.value = "";
    confirm.value = false;
    void refresh();
  }
}
async function submit(input: HistoryInput) {
  if (running.value) return;
  pending.value = { ...input };
  running.value = true;
  unknown.value = false;
  error.value = "";
  const c = (controller = new AbortController());
  try {
    const r = await api.import({ ...input }, c.signal, (r) => {
      if (!disposed && !c.signal.aborted) receipt.value = r;
    });
    if (!disposed && !c.signal.aborted) {
      accept(r);
      void refresh();
    }
  } catch {
    if (!disposed && pending.value && !c.signal.aborted) unknown.value = true;
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
    if (!disposed && !c.signal.aborted) {
      accept(r);
      error.value = "";
      if (r.state !== "pending") void refresh();
    }
  } catch {
    if (!disposed && !c.signal.aborted)
      error.value = t("backupHistory.notFound");
  } finally {
    if (!disposed && readController === c) reconciling.value = false;
  }
}
function choose(r: HistoryReceipt) {
  if (pending.value && pending.value.request_id !== r.request_id) {
    section.value = "import";
    return;
  }
  accept(r);
  section.value = "import";
}
function stop() {
  controller?.abort();
  running.value = false;
  unknown.value = true;
}
function resetIdentity() {
  if (owner === identity()) return;
  owner = identity();
  controller?.abort();
  previewController?.abort();
  recentController?.abort();
  readController?.abort();
  preview.value = undefined;
  sourceFresh.value = false;
  sourceError.value = "";
  historyError.value = "";
  error.value = "";
  receipt.value = undefined;
  receipts.value = [];
  pending.value = undefined;
  reason.value = "";
  confirm.value = false;
  unknown.value = false;
  running.value = false;
  reconciling.value = false;
  section.value = "inventory";
  void refresh();
}
const events = useBackupEvents("backup_cleanup", () => void refresh());
onMounted(() => {
  void refresh();
  window.addEventListener(AUTH_SESSION_EVENT, resetIdentity);
  window.addEventListener("storage", resetIdentity);
});
onBeforeUnmount(() => {
  disposed = true;
  controller?.abort();
  previewController?.abort();
  recentController?.abort();
  readController?.abort();
  window.removeEventListener(AUTH_SESSION_EVENT, resetIdentity);
  window.removeEventListener("storage", resetIdentity);
});
</script>
