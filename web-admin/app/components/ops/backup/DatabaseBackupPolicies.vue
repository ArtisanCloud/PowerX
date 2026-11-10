<template>
  <div class="min-w-0 space-y-4" data-testid="database-backup-policies">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="font-semibold">{{ t("storageGov.databaseBackup.title") }}</h3>
      <div class="flex gap-2">
        <UButton
          color="neutral"
          variant="outline"
          :loading="loading"
          :disabled="writing"
          @click="load"
          >{{ t("storageGov.refresh") }}</UButton
        ><UButton
          :disabled="!editable || !!unknown"
          data-testid="db-policy-new"
          @click="edit()"
          >{{ t("backupCenter.newPolicy") }}</UButton
        >
      </div>
    </div>
    <UAlert v-if="error" color="error" :title="error" /><UAlert
      v-if="events.status.value === 'disconnected'"
      color="warning"
      :title="t('storageGov.databaseBackup.disconnected')"
    />
    <div
      class="rounded-xl border border-default bg-elevated/40 p-4 text-sm"
      data-testid="db-policy-versions"
    >
      <p>
        {{
          t("storageGov.tableMonitor.control.versions", {
            desired: configuration?.desired_revision ?? "—",
            applied: configuration?.applied_revision ?? "—",
            snapshot: configuration?.snapshot_revision ?? "—",
            state: t(
              "storageGov.tableMonitor.control.states." +
                (configuration?.state || "unavailable"),
            ),
          })
        }}
      </p>
      <p class="mt-2 text-muted">{{ t("storageGov.databaseBackup.scope") }}</p>
    </div>
    <div v-if="unknown" class="space-y-3" data-testid="db-policy-unknown">
      <UAlert
        color="warning"
        :title="t('storageGov.tableMonitor.control.unknown')"
        :description="t('storageGov.tableMonitor.control.unknownHint')"
      /><UButton
        :loading="writing"
        data-testid="db-policy-reconcile"
        @click="reconcile"
        >{{ t("storageGov.tableMonitor.control.reconcile") }}</UButton
      >
    </div>
    <div v-if="configuration?.authority === 'legacy'" class="space-y-3">
      <UAlert
        color="warning"
        :title="t('storageGov.databaseBackup.legacy')"
        :description="t('storageGov.databaseBackup.adoptHint')"
      /><UCheckbox
        v-model="adoptionConfirmed"
        :disabled="writing || !!unknown"
        :label="t('storageGov.databaseBackup.adoptConfirm')"
        data-testid="db-policy-adopt-confirm"
      /><UButton
        :disabled="
          !configuration.ready || !adoptionConfirmed || writing || !!unknown
        "
        data-testid="db-policy-adopt"
        @click="adopt"
        >{{ t("storageGov.databaseBackup.adopt") }}</UButton
      >
    </div>
    <UAlert
      v-else-if="!configuration || configuration.authority !== 'database'"
      color="warning"
      :title="t('storageGov.tableMonitor.control.upgrade')"
    />
    <template v-else>
      <UAlert
        v-if="savedRevision"
        :color="
          configuration.state === 'applied' &&
          configuration.desired_revision === savedRevision
            ? 'success'
            : 'info'
        "
        :title="
          t(
            configuration.state === 'applied' &&
              configuration.desired_revision === savedRevision
              ? 'storageGov.tableMonitor.control.applied'
              : 'storageGov.databaseBackup.saved',
          )
        "
      />
      <p
        v-if="!configuration.policies.length"
        class="rounded-lg border border-dashed border-default p-6 text-center text-muted"
      >
        {{ t("backupCenter.noPolicies") }}
      </p>
      <article
        v-for="p in configuration.policies"
        :key="p.uuid"
        :data-db-policy="p.uuid"
        class="rounded-xl border border-default p-4 sm:p-5"
      >
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="min-w-0">
            <h4 class="break-words font-semibold">{{ p.settings.name }}</h4>
            <p class="mt-1 text-xs text-muted">
              {{
                t("storageGov.databaseBackup.rowVersion", {
                  version: p.revision,
                })
              }}
            </p>
          </div>
          <div class="flex flex-wrap gap-2">
            <UBadge v-if="p.current" color="info" variant="subtle">{{
              t("storageGov.resourceConfig.currentPlan")
            }}</UBadge
            ><UBadge
              :color="
                configuration.execution_ready[p.uuid] ? 'success' : 'neutral'
              "
              variant="subtle"
              >{{
                t(
                  configuration.execution_ready[p.uuid]
                    ? "storageGov.databaseBackup.executable"
                    : p.enabled
                      ? "storageGov.databaseBackup.blocked"
                      : "storageGov.archive.disabled",
                )
              }}</UBadge
            >
          </div>
        </div>
        <dl class="mt-4 grid gap-3 text-sm sm:grid-cols-3">
          <div>
            <dt class="text-muted">
              {{ t("storageGov.resourceConfig.frequency") }}
            </dt>
            <dd>{{ frequency(p.settings.schedule) }}</dd>
          </div>
          <div>
            <dt class="text-muted">
              {{ t("storageGov.resourceConfig.keepCount") }}
            </dt>
            <dd>{{ p.settings.retention_count }}</dd>
          </div>
          <div>
            <dt class="text-muted">
              {{ t("storageGov.resourceConfig.keepDays") }}
            </dt>
            <dd>
              {{
                p.settings.retention_mode === "count"
                  ? t("storageGov.resourceConfig.countOnly")
                  : p.settings.retention_days
              }}
            </dd>
          </div>
        </dl>
        <div class="mt-4 flex flex-wrap gap-2 border-t border-default pt-3">
          <UButton
            color="neutral"
            variant="outline"
            :disabled="!editable || !!unknown"
            data-testid="db-policy-edit"
            @click="edit(p)"
            >{{ t("backupCenter.edit") }}</UButton
          ><UButton
            :disabled="!editable || !!unknown"
            :color="p.enabled ? 'warning' : 'primary'"
            data-testid="db-policy-toggle"
            @click="confirmAction(p, 'status')"
            >{{
              t(
                p.enabled
                  ? "storageGov.resourceConfig.disableSchedule"
                  : "storageGov.resourceConfig.enableSchedule",
              )
            }}</UButton
          ><UButton
            v-if="!p.current"
            color="neutral"
            variant="outline"
            :disabled="!editable || !p.enabled || !!unknown"
            data-testid="db-policy-current"
            @click="confirmAction(p, 'current')"
            >{{ t("storageGov.resourceConfig.makeCurrent") }}</UButton
          >
        </div>
      </article>
    </template>
    <p class="text-xs text-muted">
      {{ t("storageGov.databaseBackup.restoreBoundary") }}
    </p>
    <UModal
      v-model:open="editorOpen"
      :title="t('backupCenter.editPolicy')"
      :description="t('storageGov.databaseBackup.editHint')"
      :dismissible="!writing && !unknown"
      :close="writing || unknown ? false : undefined"
      :ui="{ content: 'max-w-2xl' }"
      ><template #body
        ><div class="space-y-4">
          <p class="text-sm text-muted">
            {{
              t("storageGov.databaseBackup.editing", { version: baseRevision })
            }}
          </p>
          <p
            v-if="baseRevision !== configuration?.desired_revision"
            class="text-sm text-warning"
            data-testid="db-policy-conflict"
          >
            {{
              t("storageGov.tableMonitor.control.staleDraft", {
                version: baseRevision,
              })
            }}
          </p>
          <UAlert v-if="error" color="error" :title="error" />
          <UForm
            id="db-policy-form"
            :state="form"
            :schema="schema"
            class="space-y-4"
            @submit="save"
            ><fieldset :disabled="writing || !!unknown" class="space-y-4">
              <UFormField name="name" :label="t('backupCenter.policyName')"
                ><UInput
                  v-model="form.name"
                  class="w-full"
                  data-testid="db-policy-name"
              /></UFormField>
              <div class="grid gap-4 sm:grid-cols-2">
                <UFormField name="interval" :label="t('backupCenter.interval')"
                  ><UInput
                    v-model="form.interval"
                    type="number"
                    min="1"
                    max="10000"
                    class="w-full"
                    data-testid="db-policy-interval" /></UFormField
                ><UFormField :label="t('storageGov.resourceConfig.unit')"
                  ><USelect
                    v-model="form.unit"
                    :items="units"
                    class="w-full"
                    :portal="false"
                    data-testid="db-policy-unit" /></UFormField
                ><UFormField
                  name="count"
                  :label="t('backupCenter.retentionCount')"
                  ><UInput
                    v-model="form.count"
                    type="number"
                    min="1"
                    max="10000"
                    class="w-full" /></UFormField
                ><UFormField :label="t('backupCenter.retentionMode')"
                  ><USelect
                    v-model="form.mode"
                    :items="modes"
                    class="w-full"
                    :portal="false" /></UFormField
                ><UFormField
                  v-if="form.mode === 'age_and_count'"
                  name="days"
                  :label="t('backupCenter.retentionDays')"
                  ><UInput
                    v-model="form.days"
                    type="number"
                    min="1"
                    max="3650"
                    class="w-full" /></UFormField
                ><UFormField name="timezone" :label="t('backupCenter.timezone')"
                  ><UInput v-model="form.timezone" class="w-full"
                /></UFormField>
              </div></fieldset></UForm
          ><UButton
            color="neutral"
            variant="ghost"
            :disabled="writing || !!unknown"
            data-testid="db-policy-latest"
            @click="loadLatest"
            >{{ t("storageGov.tableMonitor.control.loadLatest") }}</UButton
          ><UAlert
            v-if="unknown"
            color="warning"
            :title="t('storageGov.tableMonitor.control.unknown')"
            :description="t('storageGov.tableMonitor.control.unknownHint')"
          /><UButton
            v-if="unknown"
            data-testid="db-policy-reconcile-editor"
            :disabled="writing"
            @click="reconcile"
            >{{ t("storageGov.tableMonitor.control.reconcile") }}</UButton
          >
        </div></template
      ><template #footer
        ><UButton
          type="submit"
          form="db-policy-form"
          :loading="writing"
          :disabled="!editable || !!unknown"
          data-testid="db-policy-save"
          >{{ t("storageGov.databaseBackup.savePaused") }}</UButton
        ></template
      ></UModal
    >
    <UModal
      :open="!!candidate"
      :title="t('storageGov.resourceConfig.confirmChange')"
      :description="t('storageGov.databaseBackup.statusHint')"
      :dismissible="!writing && !unknown"
      :close="writing || unknown ? false : undefined"
      @update:open="
        (value) => {
          if (!value) candidate = undefined;
        }
      "
      ><template #body
        ><div class="space-y-4">
          <p>{{ candidate?.name }}</p>
          <p class="text-sm text-muted">
            {{
              t("storageGov.databaseBackup.editing", {
                version: candidate?.revision,
              })
            }}
          </p>
          <UAlert v-if="error" color="error" :title="error" /><UCheckbox
            v-model="statusConfirmed"
            :label="t('storageGov.resourceConfig.changeHint')"
            data-testid="db-policy-status-confirm"
          /><UButton
            :disabled="writing || !!unknown || !statusConfirmed"
            data-testid="db-policy-status-save"
            @click="changeStatus"
            >{{ t("backupCenter.confirm") }}</UButton
          ><UAlert
            v-if="unknown"
            color="warning"
            :title="t('storageGov.tableMonitor.control.unknown')"
            :description="t('storageGov.tableMonitor.control.unknownHint')"
          /><UButton
            v-if="unknown"
            data-testid="db-policy-reconcile-status"
            :disabled="writing"
            @click="reconcile"
            >{{ t("storageGov.tableMonitor.control.reconcile") }}</UButton
          >
        </div></template
      ></UModal
    >
  </div>
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
import * as v from "valibot";
import {
  useDatabaseBackupPolicyService,
  type DatabaseBackupConfiguration,
  type DatabaseBackupEntry,
  type DatabaseBackupSettings,
  type DatabaseBackupChange,
  type DatabaseBackupMutation,
} from "~/composables/api/services/databaseBackupPolicyService";
import { useBackupEvents } from "~/composables/useBackupEvents";
const emit = defineEmits<{
    busy: [boolean];
    changed: [];
    attention: [boolean];
  }>(),
  { t } = useI18n(),
  api = useDatabaseBackupPolicyService();
const configuration = ref<DatabaseBackupConfiguration>(),
  loading = ref(false),
  writing = ref(false),
  error = ref(""),
  adoptionConfirmed = ref(false),
  savedRevision = ref(0);
const editorOpen = ref(false),
  editing = ref(""),
  baseRevision = ref(0),
  rowRevision = ref(0),
  statusConfirmed = ref(false);
const form = reactive({
  name: "",
  interval: 1 as number | string,
  unit: "h",
  count: 168 as number | string,
  days: 7 as number | string,
  mode: "age_and_count" as "count" | "age_and_count",
  timezone: "Asia/Shanghai",
});
type Draft = { revision: number; rowRevision: number; form: typeof form };
const drafts = new Map<string, Draft>();
const candidate = ref<{
  name: string;
  revision: number;
  change: DatabaseBackupChange;
}>();
type Pending = {
  key: string;
  revision: number;
  policy: string;
  kind: "save" | "status" | "adopt";
};
const unknown = ref<Pending>();
let reader: AbortController | undefined, writer: AbortController | undefined;
const editable = computed(
  () =>
    configuration.value?.authority === "database" &&
    configuration.value.ready &&
    !writing.value,
);
const units = computed(() =>
    ["m", "h", "d"].map((value, i) => ({
      value,
      label: t(
        "storageGov.resourceConfig.units." + ["minute", "hour", "day"][i],
      ),
    })),
  ),
  modes = computed(() => [
    { value: "count", label: t("storageGov.resourceConfig.countOnly") },
    {
      value: "age_and_count",
      label: t("storageGov.resourceConfig.ageAndCount"),
    },
  ]);
const number = (max: number) =>
  v.pipe(
    v.union([v.string(), v.number()]),
    v.transform(Number),
    v.check(
      (n) => Number.isInteger(n) && n >= 1 && n <= max,
      t("storageGov.drafts.invalid"),
    ),
  );
const schema = computed(() =>
  v.object({
    name: v.pipe(v.string(), v.trim(), v.minLength(1), v.maxLength(128)),
    interval: number(10000),
    unit: v.picklist(["m", "h", "d"]),
    count: number(10000),
    days: number(3650),
    mode: v.picklist(["count", "age_and_count"]),
    timezone: v.pipe(v.string(), v.minLength(1), v.maxLength(64)),
  }),
);
function frequency(schedule: string) {
  const match = schedule.match(/^(\d+)([mhd])$/);
  if (!match) return schedule;
  return t(
    "backupCenter.workspace.every" +
      { m: "Minutes", h: "Hours", d: "Days" }[match[2] as "m" | "h" | "d"],
    { count: Number(match[1]) },
  );
}
function accept(c: DatabaseBackupConfiguration) {
  const old = configuration.value;
  if (
    old?.authority === "database" &&
    (c.authority === "legacy" ||
      (c.desired_revision < old.desired_revision &&
        !["unconfirmed", "unavailable"].includes(c.state)))
  ) {
    configuration.value = {
      ...old,
      authority: "unavailable",
      ready: false,
      state: "unconfirmed",
    };
    return;
  }
  configuration.value = c;
}
async function load() {
  reader?.abort();
  const r = (reader = new AbortController());
  loading.value = true;
  try {
    const c = await api.configuration(r.signal);
    if (!r.signal.aborted) accept(c);
    return true;
  } catch (e) {
    if (!r.signal.aborted) {
      error.value =
        e instanceof Error ? e.message : t("storageGov.requestFailed");
      if (configuration.value)
        configuration.value = { ...configuration.value, ready: false };
    }
    return false;
  } finally {
    if (!r.signal.aborted) loading.value = false;
  }
}
const events = useBackupEvents("backup_policies", () => void load());
function edit(p?: DatabaseBackupEntry) {
  if (writing.value || unknown.value) return;
  editing.value = p?.uuid || "";
  const key = p?.uuid || "new",
    draft = drafts.get(key),
    m = p?.settings.schedule.match(/^(\d+)([mhd])$/);
  baseRevision.value =
    draft?.revision ?? configuration.value?.desired_revision ?? 0;
  rowRevision.value = draft?.rowRevision ?? p?.revision ?? 0;
  Object.assign(
    form,
    draft?.form || {
      name: p?.settings.name || "",
      interval: m ? Number(m[1]) : 1,
      unit: m?.[2] || "h",
      count: p?.settings.retention_count || 168,
      days: p?.settings.retention_days || 7,
      mode: p?.settings.retention_mode || "age_and_count",
      timezone: p?.settings.timezone || "Asia/Shanghai",
    },
  );
  error.value = "";
  editorOpen.value = true;
}
async function loadLatest() {
  if (writing.value || unknown.value) return;
  error.value = "";
  if (await load()) {
    drafts.delete(editing.value || "new");
    edit(configuration.value?.policies.find((p) => p.uuid === editing.value));
  }
}
function receipt(m: DatabaseBackupMutation, p: Pending) {
  if (
    m.request_id !== p.key ||
    m.operation_revision !== p.revision ||
    (p.policy && m.policy_uuid !== p.policy)
  )
    throw new Error("BACKUP_POLICY_RESPONSE_INVALID");
  accept(m.configuration);
  savedRevision.value = m.operation_revision;
  unknown.value = undefined;
  if (p.kind === "save") {
    drafts.delete(editing.value || "new");
    editorOpen.value = false;
  }
  candidate.value = undefined;
  adoptionConfirmed.value = false;
  emit("changed");
}
async function submit(
  p: Pending,
  action: (signal: AbortSignal) => Promise<DatabaseBackupMutation>,
) {
  const r = (writer = new AbortController());
  writing.value = true;
  error.value = "";
  try {
    const m = await action(r.signal);
    if (!r.signal.aborted) receipt(m, p);
  } catch (e: any) {
    if (!r.signal.aborted) {
      error.value = e?.message || t("storageGov.requestFailed");
      if (
        ![400, 401, 403, 409, 422, 428].includes(
          e?.status || e?.statusCode || e?.response?.status,
        )
      )
        unknown.value = p;
    }
  } finally {
    if (!r.signal.aborted) writing.value = false;
  }
}
async function adopt() {
  const c = configuration.value;
  if (!c?.ready || !adoptionConfirmed.value || writing.value || unknown.value)
    return;
  const key = crypto.randomUUID();
  await submit({ key, revision: 1, policy: "", kind: "adopt" }, (s) =>
    api.adopt(
      { request_id: key, legacy_digest: c.legacy_digest, confirm: true },
      s,
    ),
  );
}
async function save() {
  if (
    !editable.value ||
    unknown.value ||
    !v.safeParse(schema.value, form).success
  )
    return;
  const key = crypto.randomUUID();
  const settings: DatabaseBackupSettings = {
    name: form.name.trim(),
    schedule: String(form.interval) + form.unit,
    retention_count: Number(form.count),
    retention_days: Number(form.days),
    retention_mode: form.mode,
    timezone: form.timezone,
    drill_enabled: false,
    drill_interval_days: 7,
    target_ref: "local_dump",
  };
  const change: DatabaseBackupChange = {
    request_id: key,
    expected_revision: baseRevision.value,
    expected_policy_revision: rowRevision.value,
    policy_uuid: editing.value,
    action: "save",
    settings,
    confirm: true,
  };
  await submit(
    {
      key,
      revision: baseRevision.value + 1,
      policy: editing.value || key,
      kind: "save",
    },
    (s) => api.change(change, s),
  );
}
function confirmAction(p: DatabaseBackupEntry, action: "status" | "current") {
  const c = configuration.value;
  if (!c || !editable.value || unknown.value) return;
  statusConfirmed.value = false;
  error.value = "";
  candidate.value = {
    name: p.settings.name,
    revision: c.desired_revision,
    change: {
      request_id: crypto.randomUUID(),
      expected_revision: c.desired_revision,
      expected_policy_revision: p.revision,
      policy_uuid: p.uuid,
      action,
      enabled: action === "status" ? !p.enabled : undefined,
      confirm: true,
    },
  };
}
async function changeStatus() {
  const c = candidate.value;
  if (!c || !statusConfirmed.value || writing.value || unknown.value) return;
  await submit(
    {
      key: c.change.request_id,
      revision: c.revision + 1,
      policy: c.change.policy_uuid,
      kind: "status",
    },
    (s) => api.change(c.change, s),
  );
}
async function reconcile() {
  const p = unknown.value;
  if (!p || writing.value) return;
  const r = (writer = new AbortController());
  writing.value = true;
  try {
    const m = await api.request(p.key, r.signal);
    if (!r.signal.aborted) {
      receipt(m, p);
      error.value = "";
    }
  } catch {
    if (!r.signal.aborted)
      error.value = t("storageGov.tableMonitor.control.unknownHint");
  } finally {
    if (!r.signal.aborted) writing.value = false;
  }
}
watch(
  form,
  () => {
    if (editorOpen.value)
      drafts.set(editing.value || "new", {
        revision: baseRevision.value,
        rowRevision: rowRevision.value,
        form: { ...form },
      });
  },
  { deep: true },
);
watch(unknown, (value) => emit("attention", !!value));
watch(
  () => writing.value || !!unknown.value,
  (b) => emit("busy", b),
);
onMounted(load);
onBeforeUnmount(() => {
  reader?.abort();
  writer?.abort();
  emit("busy", false);
});
</script>
