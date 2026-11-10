<template>
  <section class="min-w-0 space-y-4" data-testid="quarantine-inventory">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold">
          {{ t("quarantineInventory.title") }}
        </h2>
        <p class="mt-1 text-sm text-muted">
          {{ t("quarantineInventory.hint") }}
        </p>
      </div>
      <UButton
        color="neutral"
        variant="outline"
        :loading="loading"
        data-testid="inventory-refresh"
        @click="load()"
        >{{ t("backupReview.refresh") }}</UButton
      >
    </header>
    <UAlert v-if="error" color="warning" :title="error" />
    <UAlert
      v-if="events.status.value === 'disconnected'"
      color="warning"
      :title="t('backupQuarantine.disconnected')"
    />
    <UAlert
      v-if="unattributed"
      color="warning"
      :title="t('quarantineInventory.coverageGap', { count: unattributed })"
    />
    <form
      class="grid gap-3 rounded-xl border border-default p-4 sm:grid-cols-2 xl:grid-cols-3"
      @submit.prevent="apply"
    >
      <UFormField :label="t('quarantineInventory.search')"
        ><UInput
          v-model="draft.q"
          :placeholder="t('quarantineInventory.searchHint')"
          :maxlength="64"
          class="w-full"
          data-testid="inventory-query"
      /></UFormField>
      <UFormField :label="t('quarantineInventory.currentLabel')"
        ><USelect
          v-model="draft.current"
          :items="currentOptions"
          class="w-full"
          data-testid="inventory-current-filter"
      /></UFormField>
      <UFormField :label="t('quarantineInventory.resultLabel')"
        ><USelect v-model="draft.result" :items="resultOptions" class="w-full"
      /></UFormField>
      <UFormField :label="t('quarantineInventory.diagnosisLabel')"
        ><USelect
          v-model="draft.diagnosis"
          :items="diagnosisOptions"
          class="w-full"
      /></UFormField>
      <UFormField :label="t('quarantineInventory.sortLabel')"
        ><div class="flex gap-2">
          <USelect
            v-model="draft.sort"
            :items="sortOptions"
            class="min-w-0 flex-1"
          /><UButton
            type="button"
            color="neutral"
            variant="outline"
            @click="
              draft.direction = draft.direction === 'desc' ? 'asc' : 'desc'
            "
            >{{ t("quarantineInventory." + draft.direction) }}</UButton
          >
        </div></UFormField
      >
      <div class="flex items-end gap-2">
        <UButton
          type="submit"
          :disabled="loading"
          data-testid="inventory-apply"
          >{{ t("quarantineInventory.apply") }}</UButton
        ><UButton
          type="button"
          color="neutral"
          variant="ghost"
          @click="reset"
          >{{ t("quarantineInventory.reset") }}</UButton
        >
      </div>
    </form>
    <p v-if="loaded" class="text-sm text-muted">
      {{
        t("quarantineInventory.observed", {
          count: total,
          time: time(observed),
        })
      }}
    </p>
    <p v-else-if="loading" role="status">{{ t("backupReview.loading") }}</p>
    <div
      v-if="loaded && !items.length"
      class="rounded-xl border border-default p-8 text-center text-muted"
      data-testid="inventory-empty"
    >
      {{ t("quarantineInventory.empty") }}
    </div>
    <div
      v-else-if="loaded"
      class="overflow-x-auto rounded-xl border border-default"
    >
      <table
        class="hidden w-full text-left text-sm sm:table"
        data-testid="inventory-table"
      >
        <thead class="bg-elevated">
          <tr>
            <th class="p-4">{{ t("quarantineInventory.artifact") }}</th>
            <th class="p-4">{{ t("quarantineInventory.currentLabel") }}</th>
            <th class="p-4">{{ t("quarantineInventory.resultLabel") }}</th>
            <th class="p-4">{{ t("quarantineInventory.time") }}</th>
            <th class="p-4">{{ t("backupReview.actions") }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-default">
          <tr v-for="row in items" :key="row.request_id">
            <td class="p-4 align-top">
              <p class="font-medium">
                #{{ row.artifact_id }} ·
                {{ t("backupReview.job", { id: row.job_id }) }}
              </p>
              <p class="mt-1 text-xs text-muted">{{ bytes(row.size_bytes) }}</p>
            </td>
            <td class="p-4 align-top">
              <UBadge
                :color="row.current === 'quarantined' ? 'warning' : 'neutral'"
                variant="subtle"
                >{{ t("quarantineInventory.current." + row.current) }}</UBadge
              >
              <p
                v-if="row.history_status !== 'covered'"
                class="mt-2 text-xs text-warning"
              >
                {{ t("quarantineInventory.history." + row.history_status) }}
              </p>
            </td>
            <td class="p-4 align-top">
              <p>{{ t("quarantineInventory.results." + row.result) }}</p>
              <p class="mt-1 text-xs text-muted">
                {{ t("quarantineInventory.diagnoses." + row.diagnosis) }}
              </p>
            </td>
            <td class="whitespace-nowrap p-4 align-top text-muted">
              {{ time(row.completed_at || row.created_at) }}
            </td>
            <td class="p-4 align-top">
              <UButton
                size="sm"
                color="neutral"
                variant="outline"
                :data-inventory-id="row.request_id"
                @click="inspect(row)"
                >{{ t("quarantineInventory.details") }}</UButton
              >
            </td>
          </tr>
        </tbody>
      </table>
      <div class="divide-y divide-default sm:hidden">
        <article
          v-for="row in items"
          :key="row.request_id"
          class="space-y-3 p-4"
        >
          <div class="flex flex-wrap justify-between gap-2">
            <strong
              >#{{ row.artifact_id }} ·
              {{ t("backupReview.job", { id: row.job_id }) }}</strong
            ><UBadge color="warning" variant="subtle">{{
              t("quarantineInventory.current." + row.current)
            }}</UBadge>
          </div>
          <p class="text-sm">
            {{ t("quarantineInventory.results." + row.result) }} ·
            {{ t("quarantineInventory.diagnoses." + row.diagnosis) }}
          </p>
          <p class="text-xs text-muted">
            {{ bytes(row.size_bytes) }} ·
            {{ time(row.completed_at || row.created_at) }}
          </p>
          <p
            v-if="row.history_status !== 'covered'"
            class="text-xs text-warning"
          >
            {{ t("quarantineInventory.history." + row.history_status) }}
          </p>
          <UButton
            color="neutral"
            variant="outline"
            :data-inventory-card-id="row.request_id"
            @click="inspect(row)"
            >{{ t("quarantineInventory.details") }}</UButton
          >
        </article>
      </div>
    </div>
    <footer
      v-if="loaded"
      class="flex flex-wrap items-center justify-between gap-3"
    >
      <span class="text-sm text-muted">{{
        t("backupReview.page", { page: applied.page })
      }}</span>
      <div class="flex gap-2">
        <UButton
          color="neutral"
          variant="outline"
          :disabled="loading || applied.page <= 1"
          @click="load(applied.page - 1)"
          >{{ t("backupCenter.previous") }}</UButton
        ><UButton
          color="neutral"
          variant="outline"
          :disabled="loading || applied.page * 20 >= total"
          @click="load(applied.page + 1)"
          >{{ t("backupCenter.next") }}</UButton
        >
      </div>
    </footer>
    <UModal
      v-model:open="open"
      :title="t('quarantineInventory.detailTitle')"
      :description="t('quarantineInventory.readOnlyHint')"
      :ui="{ content: 'w-[calc(100%-2rem)] max-w-4xl' }"
      ><template #body>
        <div
          v-if="selected"
          class="min-w-0 space-y-4"
          data-testid="inventory-detail"
        >
          <p class="font-medium">
            #{{ selected.artifact_id }} ·
            {{ t("backupReview.job", { id: selected.job_id }) }}
          </p>
          <UBadge color="warning" variant="subtle">{{
            t("quarantineInventory.current." + selected.current)
          }}</UBadge>
          <UAlert v-if="detailError" color="warning" :title="detailError" />
          <UAlert
            v-if="!detailFresh"
            color="warning"
            :title="t('quarantineInventory.stale')"
          />
          <BackupSectionTabs
            v-model="section"
            :items="detailSections"
            :label="t('quarantineInventory.detailTitle')"
          >
            <template #history
              ><section
                class="min-w-0 space-y-4"
                data-testid="inventory-history"
              >
                <h3 class="font-semibold">
                  {{ t("quarantineInventory.resultLabel") }} ·
                  {{ t("quarantineInventory.results." + selected.result) }}
                </h3>
                <p class="text-sm text-muted">
                  {{ t("quarantineInventory.historicalOnly") }}
                </p>
                <dl
                  class="space-y-3 rounded-xl border border-default p-4 text-sm"
                >
                  <div>
                    <dt class="text-muted">
                      {{ t("quarantineInventory.request") }}
                    </dt>
                    <dd
                      class="mt-1 break-all font-mono text-xs"
                      data-testid="inventory-request-id"
                    >
                      {{ selected.request_id }}
                    </dd>
                  </div>
                  <div>
                    <dt class="text-muted">
                      {{ t("quarantineInventory.token") }}
                    </dt>
                    <dd class="mt-1 break-all font-mono text-xs">
                      {{ selected.token }}
                    </dd>
                  </div>
                  <div>
                    <dt class="text-muted">
                      {{ t("quarantineInventory.reason") }}
                    </dt>
                    <dd class="mt-1 break-words">{{ selected.reason }}</dd>
                  </div>
                  <div>
                    <dt class="text-muted">
                      {{ t("quarantineInventory.diagnosisLabel") }}
                    </dt>
                    <dd>
                      {{
                        t("quarantineInventory.diagnoses." + selected.diagnosis)
                      }}
                    </dd>
                  </div>
                  <div>
                    <dt class="text-muted">
                      {{ t("quarantineInventory.time") }}
                    </dt>
                    <dd>
                      {{ time(selected.completed_at || selected.created_at) }} ·
                      {{
                        t("quarantineInventory.actor", {
                          id: selected.actor_id,
                        })
                      }}
                    </dd>
                  </div>
                  <div>
                    <dt class="text-muted">
                      {{ t("quarantineInventory.historyLabel") }}
                    </dt>
                    <dd>
                      {{
                        t(
                          "quarantineInventory.history." +
                            selected.history_status,
                        )
                      }}
                    </dd>
                  </div>
                </dl>
              </section></template
            >
            <template #inspection
              ><section
                class="min-w-0 space-y-4"
                data-testid="inventory-inspection"
              >
                <h3 class="font-semibold">
                  {{ t("quarantineInventory.inspectionTitle") }}
                </h3>
                <p class="text-sm text-muted">
                  {{ t("quarantineInventory.readOnlyHint") }}
                </p>
                <UAlert
                  v-if="!selected.can_inspect"
                  color="warning"
                  :title="t('quarantineInventory.cannotInspect')"
                />
                <div v-if="running" role="status">
                  <p class="text-sm">
                    {{ t("backupQuarantine.running") }} ·
                    {{ bytes(progress?.bytes_read || 0) }} /
                    {{ bytes(progress?.total_bytes || 0) }}
                  </p>
                  <progress
                    class="h-2 w-full accent-primary"
                    :value="progress?.bytes_read || 0"
                    :max="Math.max(progress?.total_bytes || 1, 1)"
                  />
                </div>
                <UAlert
                  v-if="inspectionError"
                  color="warning"
                  :title="inspectionError"
                /><UAlert
                  v-if="result && !running"
                  color="info"
                  :title="
                    t('backupReview.verify.states.' + result.verification.state)
                  "
                  :description="t('quarantineInventory.noRestoreProof')"
                  data-testid="inventory-inspection-result"
                />
                <div class="flex flex-wrap gap-2">
                  <UButton
                    :disabled="
                      !detailFresh ||
                      !selected.can_inspect ||
                      detailLoading ||
                      running
                    "
                    data-testid="inventory-inspect"
                    @click="diagnose"
                    >{{ t("quarantineInventory.start") }}</UButton
                  ><UButton
                    v-if="running"
                    color="neutral"
                    variant="outline"
                    @click="stop"
                    >{{ t("backupQuarantine.stopDiagnosis") }}</UButton
                  >
                </div>
              </section></template
            >
          </BackupSectionTabs>
        </div> </template
      ><template #footer
        ><div class="flex w-full flex-wrap justify-end gap-2">
          <UButton
            color="neutral"
            variant="outline"
            :loading="detailLoading"
            data-testid="inventory-read-current"
            @click="readSelected"
            >{{ t("quarantineInventory.readCurrent") }}</UButton
          ><UButton color="neutral" @click="close">{{
            t("backupReview.close")
          }}</UButton>
        </div></template
      ></UModal
    >
  </section>
</template>
<script setup lang="ts">
import {
  computed,
  onBeforeUnmount,
  onMounted,
  reactive,
  ref,
  watch,
} from "vue";
import { useI18n } from "vue-i18n";
import BackupSectionTabs from "./BackupSectionTabs.vue";
import {
  useQuarantineInventoryService,
  type QuarantineInventoryOptions,
  type QuarantineInventoryItem,
  type QuarantineInspection,
} from "~/composables/api/services/quarantineInventoryService";
import type { BackupCleanupVerification } from "~/composables/api/services/backupCleanupReviewService";
import { useBackupEvents } from "~/composables/useBackupEvents";
import { AUTH_SESSION_EVENT, readAuthSession } from "~/utils/auth-session";
const emit = defineEmits<{
  attention: [value: { count: number; problem: boolean }];
}>();
const { t } = useI18n(),
  api = useQuarantineInventoryService();
const defaults = (): QuarantineInventoryOptions => ({
  page: 1,
  page_size: 20,
  current: "all",
  result: "all",
  diagnosis: "all",
  q: "",
  sort: "time",
  direction: "desc",
});
const draft = reactive(defaults()),
  applied = ref(defaults()),
  items = ref<QuarantineInventoryItem[]>([]),
  total = ref(0),
  unattributed = ref(0),
  observed = ref(""),
  loaded = ref(false),
  loading = ref(false),
  error = ref("");
const open = ref(false),
  selected = ref<QuarantineInventoryItem>(),
  section = ref("history"),
  detailFresh = ref(false),
  detailLoading = ref(false),
  detailError = ref(""),
  running = ref(false),
  inspectionError = ref(""),
  progress = ref<BackupCleanupVerification>(),
  result = ref<QuarantineInspection>();
const choices = (values: string[], prefix: string) =>
  values.map((value) => ({ value, label: t(prefix + "." + value) }));
const currentOptions = computed(() =>
  choices(
    [
      "all",
      "quarantined",
      "not_quarantined",
      "missing",
      "unavailable",
      "identity_changed",
    ],
    "quarantineInventory.current",
  ),
);
const resultOptions = computed(() =>
  choices(
    ["all", "checking", "quarantined", "rejected"],
    "quarantineInventory.results",
  ),
);
const diagnosisOptions = computed(() => [
  { value: "all", label: t("quarantineInventory.allDiagnosis") },
  ...choices(
    [
      "missing",
      "size_changed",
      "checksum_mismatch",
      "sidecars_missing",
      "sidecar_mismatch",
      "metadata_invalid",
    ],
    "quarantineInventory.diagnoses",
  ),
]);
const sortOptions = computed(() =>
  choices(["time", "size", "artifact"], "quarantineInventory.sort"),
);
const detailSections = computed(() => [
  { value: "history", label: t("quarantineInventory.historyTab") },
  { value: "inspection", label: t("quarantineInventory.inspectionTitle") },
]);
const time = (v: string) => (v ? new Date(v).toLocaleString() : "—");
const bytes = (v: number | null) =>
  v === null || v < 0
    ? t("backupReview.unknownSize")
    : v < 1024
      ? `${v} B`
      : v < 1048576
        ? `${(v / 1024).toFixed(1)} KiB`
        : `${(v / 1048576).toFixed(1)} MiB`;
let listController: AbortController | undefined,
  detailController: AbortController | undefined,
  inspectionController: AbortController | undefined,
  disposed = false;
const identity = () => {
  const s = readAuthSession();
  return `${s?.sessionId || ""}:${s?.tenantUuid || ""}:${s?.status || ""}`;
};
let owner = identity();
async function load(page = applied.value.page, input = applied.value) {
  listController?.abort();
  const c = (listController = new AbortController());
  loading.value = true;
  const options = { ...input, page };
  try {
    const v = await api.list(options, c.signal);
    if (c.signal.aborted || disposed) return;
    items.value = v.items;
    total.value = v.total;
    unattributed.value = v.unattributed_quarantined_artifacts;
    emit("attention", { count: unattributed.value, problem: false });
    observed.value = v.observed_at;
    applied.value = options;
    loaded.value = true;
    error.value = "";
  } catch {
    if (!c.signal.aborted && !disposed) {
      error.value = t("quarantineInventory.loadFailed");
      emit("attention", { count: unattributed.value, problem: true });
    }
  } finally {
    if (listController === c && !disposed) loading.value = false;
  }
}
function apply() {
  void load(1, { ...draft, page: 1, q: draft.q.trim() });
}
function reset() {
  Object.assign(draft, defaults());
  apply();
}
function inspect(row: QuarantineInventoryItem) {
  stop();
  inspectionError.value = "";
  result.value = undefined;
  progress.value = undefined;
  selected.value = { ...row };
  section.value = "history";
  open.value = true;
  detailFresh.value = false;
  void readSelected();
}
async function readSelected() {
  if (!selected.value || !open.value) return;
  detailController?.abort();
  const c = (detailController = new AbortController()),
    key = selected.value.request_id;
  detailLoading.value = true;
  try {
    const r = await api.request(key, c.signal);
    if (c.signal.aborted || disposed || selected.value?.request_id !== key)
      return;
    if (selected.value.snapshot !== r.snapshot) {
      if (running.value) stop();
      result.value = undefined;
    }
    selected.value = r;
    detailFresh.value = true;
    detailError.value = "";
  } catch {
    if (!c.signal.aborted && !disposed) {
      detailFresh.value = false;
      detailError.value = t("quarantineInventory.detailFailed");
      if (running.value) stop();
    }
  } finally {
    if (detailController === c && !disposed) detailLoading.value = false;
  }
}
async function diagnose() {
  if (!selected.value?.can_inspect || !detailFresh.value || running.value)
    return;
  const row = { ...selected.value },
    c = (inspectionController = new AbortController());
  running.value = true;
  inspectionError.value = "";
  result.value = undefined;
  progress.value = undefined;
  try {
    const v = await api.inspect(row, c.signal, (p) => {
      if (!c.signal.aborted && !disposed) progress.value = p;
    });
    if (
      !c.signal.aborted &&
      !disposed &&
      selected.value?.snapshot === row.snapshot
    )
      result.value = v;
  } catch {
    if (!c.signal.aborted && !disposed) {
      inspectionError.value = t("quarantineInventory.incomplete");
      detailFresh.value = false;
    }
  } finally {
    if (inspectionController === c && !disposed) running.value = false;
  }
}
function stop() {
  inspectionController?.abort();
  if (running.value)
    inspectionError.value = t("quarantineInventory.incomplete");
  running.value = false;
}
function close() {
  (document.activeElement as HTMLElement | null)?.blur();
  open.value = false;
}
function identityChanged() {
  if (owner === identity()) return false;
  owner = identity();
  emit("attention", { count: 0, problem: false });
  listController?.abort();
  detailController?.abort();
  stop();
  open.value = false;
  selected.value = undefined;
  result.value = undefined;
  items.value = [];
  total.value = unattributed.value = 0;
  loaded.value = false;
  loading.value = detailLoading.value = false;
  error.value = detailError.value = "";
  detailFresh.value = false;
  Object.assign(draft, defaults());
  applied.value = defaults();
  if (readAuthSession()?.status === "active") void load(1);
  return true;
}
const events = useBackupEvents("backup_cleanup", async () => {
  if (identityChanged()) return;
  await Promise.all([load(), readSelected()]);
});
watch(open, (v) => {
  if (!v) {
    stop();
    detailController?.abort();
    detailLoading.value = false;
  }
});
onMounted(() => {
  void load();
  window.addEventListener(AUTH_SESSION_EVENT, identityChanged);
  window.addEventListener("storage", identityChanged);
});
onBeforeUnmount(() => {
  disposed = true;
  listController?.abort();
  detailController?.abort();
  inspectionController?.abort();
  window.removeEventListener(AUTH_SESSION_EVENT, identityChanged);
  window.removeEventListener("storage", identityChanged);
});
defineExpose({
  refresh: async () => {
    await Promise.all([load(), readSelected()]);
  },
});
</script>
