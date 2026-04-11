<template>
  <section class="mx-auto max-w-6xl space-y-6 p-6">
    <header>
      <h1 class="text-2xl font-semibold">备份恢复中心</h1>
      <p class="mt-1 text-sm text-gray-500">策略管理、任务执行、恢复演练与日志观测。</p>
      <p v-if="permissionHint" class="text-xs text-amber-600">{{ permissionHint }}</p>
    </header>

    <div class="grid gap-4 md:grid-cols-2">
      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <h3 class="mb-3 text-sm font-medium text-gray-700">策略管理</h3>
        <div class="grid gap-2">
          <input v-model="policyForm.name" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="策略名称" />
          <input v-model.number="policyForm.intervalHours" type="number" min="1" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="间隔小时（默认 6）" />
          <input v-model.number="policyForm.retentionCount" type="number" min="1" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="保留份数（默认 14）" />
          <input v-model="policyForm.timezone" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="时区（默认 Asia/Shanghai）" />
          <input v-model.number="policyForm.drillIntervalDays" type="number" min="1" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="演练周期天数（默认 7）" />
          <input v-model="policyForm.targetRef" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="目标库/实例（默认 powerx_bak）" />
          <label class="inline-flex items-center gap-2 text-sm text-gray-700">
            <input v-model="policyForm.drillEnabled" type="checkbox" />
            启用恢复演练
          </label>
          <p v-if="policyError" class="text-xs text-red-600">{{ policyError }}</p>
          <div class="flex gap-2">
            <button class="rounded bg-black px-4 py-2 text-sm text-white" :disabled="!canExecute" @click="savePolicy">{{ editingPolicyId ? '更新策略' : '创建策略' }}</button>
            <button v-if="editingPolicyId" class="rounded border border-gray-300 px-4 py-2 text-sm" @click="resetForm">取消编辑</button>
          </div>
        </div>
      </div>

      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <h3 class="mb-3 text-sm font-medium text-gray-700">策略列表</h3>
        <div class="mb-2 flex gap-2">
          <select v-model="filters.status" class="rounded border border-gray-300 px-3 py-2 text-sm">
            <option value="">全部状态</option>
            <option value="enabled">已启用</option>
            <option value="disabled">已停用</option>
          </select>
          <input v-model="filters.keyword" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="关键词" />
          <button class="rounded border border-gray-300 px-3 py-2 text-sm" @click="loadPolicies">筛选</button>
        </div>
        <div class="max-h-72 space-y-2 overflow-auto">
          <div v-for="p in policies" :key="String(p.id)" class="rounded border border-gray-200 p-3 text-sm">
            <div class="flex items-center justify-between">
              <strong>{{ p.name }}</strong>
              <span :class="p.enabled ? 'text-emerald-600' : 'text-gray-500'">{{ p.enabled ? '启用中' : '已停用' }}</span>
            </div>
            <p class="mt-1 text-xs text-gray-500">{{ p.interval_hours }}h / 保留 {{ p.retention_count }} 份 / {{ p.timezone }}</p>
            <p class="text-xs text-gray-500">目标：{{ p.target_ref || '-' }}，演练：{{ p.drill_enabled ? `每 ${p.drill_interval_days} 天` : '关闭' }}</p>
            <div class="mt-2 flex gap-2">
              <button class="rounded border border-gray-300 px-2 py-1 text-xs" @click="editPolicy(p)">编辑</button>
              <button v-if="!p.enabled" class="rounded border border-emerald-300 px-2 py-1 text-xs text-emerald-700" :disabled="!canExecute" @click="togglePolicy(p, true)">启用</button>
              <button v-else class="rounded border border-amber-300 px-2 py-1 text-xs text-amber-700" :disabled="!canExecute" @click="togglePolicy(p, false)">停用</button>
            </div>
          </div>
          <p v-if="policies.length === 0" class="text-xs text-gray-500">暂无策略</p>
        </div>
      </div>
    </div>

    <div class="rounded-lg border border-gray-200 bg-white p-4">
      <h3 class="mb-3 text-sm font-medium text-gray-700">执行动作</h3>
      <div class="space-y-2">
        <select v-model="selectedPolicyId" class="w-full rounded border border-gray-300 px-3 py-2 text-sm">
          <option value="">选择策略</option>
          <option v-for="p in policies" :key="String(p.id)" :value="String(p.id)">{{ p.name }}</option>
        </select>
        <button class="rounded border border-gray-300 px-4 py-2 text-sm" :disabled="!canExecute" @click="runBackup">手动触发备份</button>
        <button class="rounded border border-gray-300 px-4 py-2 text-sm" :disabled="!canExecute" @click="runCleanup">触发清理</button>
        <button class="rounded border border-gray-300 px-4 py-2 text-sm" :disabled="!latestJob || !canExecute" @click="runDrill">触发恢复演练</button>
      </div>
    </div>

    <BackupJobTable :items="jobs" />

    <div class="grid gap-4 md:grid-cols-2">
      <RestoreDrillPanel :drill="latestDrill" />
      <LogObservabilityPanel />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useBackupOpsService, type BackupJob, type BackupPolicy, type RestoreDrillRecord } from "~/composables/api/services/backupOpsService";
import BackupJobTable from "~/components/ops/backup/BackupJobTable.vue";
import RestoreDrillPanel from "~/components/ops/backup/RestoreDrillPanel.vue";
import LogObservabilityPanel from "~/components/ops/backup/LogObservabilityPanel.vue";
import { useOpsAccess } from "~/composables/useOpsAccess";

const backupSvc = useBackupOpsService();
const { canExecute, permissionHint, loadUserContext } = useOpsAccess();
const policies = ref<BackupPolicy[]>([]);
const jobs = ref<BackupJob[]>([]);
const latestDrill = ref<RestoreDrillRecord | null>(null);
const selectedPolicyId = ref("");
const editingPolicyId = ref<string>("");
const policyError = ref("");

const filters = reactive({
  status: "" as "" | "enabled" | "disabled",
  keyword: "",
});

const policyForm = reactive({
  name: "",
  intervalHours: 6,
  retentionCount: 14,
  timezone: "Asia/Shanghai",
  drillEnabled: true,
  drillIntervalDays: 7,
  targetRef: "powerx_bak",
});

const latestJob = computed(() => (jobs.value.length > 0 ? jobs.value[0] : null));

const isValidTimezone = (timezone: string): boolean => {
  try {
    Intl.DateTimeFormat("zh-CN", { timeZone: timezone });
    return true;
  } catch {
    return false;
  }
};

const validatePolicyForm = () => {
  if (!policyForm.name.trim()) return "策略名称不能为空";
  if (!Number.isFinite(policyForm.intervalHours) || policyForm.intervalHours <= 0) return "间隔小时必须大于 0";
  if (!Number.isFinite(policyForm.retentionCount) || policyForm.retentionCount <= 0) return "保留份数必须大于 0";
  const timezone = policyForm.timezone.trim();
  if (!timezone) return "时区不能为空";
  if (!isValidTimezone(timezone)) return "时区不合法，请使用 IANA 时区（例如 Asia/Shanghai）";
  if (!Number.isFinite(policyForm.drillIntervalDays) || policyForm.drillIntervalDays <= 0) return "演练周期天数必须大于 0";
  return "";
};

const loadPolicies = async () => {
  const result = await backupSvc.listPolicies({ status: filters.status || undefined, keyword: filters.keyword || undefined, page: 1, pageSize: 50 });
  policies.value = result.items;
};

const loadJobs = async () => {
  jobs.value = await backupSvc.listJobs();
};

const load = async () => {
  await Promise.all([loadPolicies(), loadJobs()]);
};

const resetForm = () => {
  editingPolicyId.value = "";
  policyError.value = "";
  policyForm.name = "";
  policyForm.intervalHours = 6;
  policyForm.retentionCount = 14;
  policyForm.timezone = "Asia/Shanghai";
  policyForm.drillEnabled = true;
  policyForm.drillIntervalDays = 7;
  policyForm.targetRef = "powerx_bak";
};

const editPolicy = (p: BackupPolicy) => {
  editingPolicyId.value = String(p.id);
  policyError.value = "";
  policyForm.name = p.name;
  policyForm.intervalHours = p.interval_hours || 6;
  policyForm.retentionCount = p.retention_count || 14;
  policyForm.timezone = p.timezone || "Asia/Shanghai";
  policyForm.drillEnabled = !!p.drill_enabled;
  policyForm.drillIntervalDays = p.drill_interval_days || 7;
  policyForm.targetRef = p.target_ref || "powerx_bak";
};

const savePolicy = async () => {
  if (!canExecute.value) return;
  policyError.value = validatePolicyForm();
  if (policyError.value) return;

  const payload = {
    name: policyForm.name.trim(),
    interval_hours: policyForm.intervalHours,
    retention_count: policyForm.retentionCount,
    timezone: policyForm.timezone.trim(),
    drill_enabled: policyForm.drillEnabled,
    drill_interval_days: policyForm.drillIntervalDays,
    target_ref: policyForm.targetRef.trim() || "powerx_bak",
  };

  try {
    if (editingPolicyId.value) {
      await backupSvc.updatePolicy(editingPolicyId.value, payload);
    } else {
      await backupSvc.createPolicy(payload);
    }
    await loadPolicies();
    resetForm();
  } catch (error: any) {
    policyError.value = error?.message || "策略保存失败，请稍后重试";
  }
};

const togglePolicy = async (p: BackupPolicy, enabled: boolean) => {
  if (!canExecute.value) return;
  if (enabled) await backupSvc.enablePolicy(p.id);
  else await backupSvc.disablePolicy(p.id);
  await loadPolicies();
};

const runBackup = async () => {
  if (!canExecute.value) return;
  if (!selectedPolicyId.value) return;
  await backupSvc.triggerJob(selectedPolicyId.value);
  await loadJobs();
};

const runCleanup = async () => {
  if (!canExecute.value) return;
  await backupSvc.triggerCleanup();
};

const runDrill = async () => {
  if (!canExecute.value) return;
  if (!latestJob.value) return;
  latestDrill.value = await backupSvc.triggerRestoreDrill(latestJob.value.id);
};

onMounted(async () => {
  await loadUserContext();
  await load();
});
</script>
