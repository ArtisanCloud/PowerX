<template>
  <section class="mx-auto max-w-6xl space-y-6 p-6">
    <header class="rounded-xl border border-gray-200 bg-white p-5">
      <h1 class="text-2xl font-semibold text-gray-900">备份恢复中心</h1>
      <p class="mt-1 text-sm text-gray-600">以策略为中心，按步骤完成备份闭环。</p>
      <p v-if="permissionHint" class="mt-2 text-xs text-amber-600">{{ permissionHint }}</p>
    </header>

    <UCard>
      <UStepper v-model="currentStep" :items="stepperItems" class="w-full" />
    </UCard>

    <UCard v-if="currentStep === 0">
      <template #header>
        <div class="flex items-center justify-between gap-3">
          <div>
            <h3 class="text-sm font-semibold text-gray-800">Step 1：策略列表（主视图）</h3>
            <p class="text-xs text-gray-500">先创建并启用策略，再进入执行动作。</p>
          </div>
          <UButton color="primary" :disabled="!canExecute" @click="openCreatePolicyModal">新建策略</UButton>
        </div>
      </template>

      <div class="mb-3 flex flex-wrap gap-2">
        <select v-model="filters.status" class="rounded border border-gray-300 px-3 py-2 text-sm">
          <option value="">全部状态</option>
          <option value="enabled">已启用</option>
          <option value="disabled">已停用</option>
        </select>
        <input v-model="filters.keyword" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="关键词" />
        <UButton color="neutral" variant="outline" @click="loadPolicies">筛选</UButton>
      </div>

      <div class="max-h-96 space-y-2 overflow-auto">
        <div
          v-for="p in policies"
          :key="String(p.id)"
          class="rounded border p-3 text-sm"
          :class="isCurrentPolicy(p) ? 'border-emerald-500/60 bg-emerald-500/5' : 'border-gray-200'"
        >
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <strong>{{ p.name }}</strong>
              <span v-if="isCurrentPolicy(p)" class="rounded bg-emerald-500/15 px-2 py-0.5 text-xs text-emerald-600">当前执行策略</span>
            </div>
            <span :class="p.enabled ? 'text-emerald-600' : 'text-gray-500'">{{ p.enabled ? "启用中" : "已停用" }}</span>
          </div>
          <p class="mt-1 text-xs text-gray-500">{{ formatPolicyInterval(p) }} / 保留 {{ p.retention_count }} 份 / {{ p.timezone }}</p>
          <p class="text-xs text-gray-500">目标：{{ p.target_ref || '-' }}，恢复任务：{{ p.drill_enabled ? `每 ${p.drill_interval_days} 天` : '关闭' }}</p>
          <div class="mt-2 flex flex-wrap gap-2">
            <UButton size="xs" color="neutral" variant="outline" @click="openEditPolicyModal(p)">编辑</UButton>
            <UButton
              v-if="!p.enabled"
              size="xs"
              color="success"
              variant="outline"
              :disabled="!canExecute"
              @click="togglePolicy(p, true)"
            >
              启用
            </UButton>
            <UButton
              v-else
              size="xs"
              color="warning"
              variant="outline"
              :disabled="!canExecute"
              @click="togglePolicy(p, false)"
            >
              停用
            </UButton>
            <UButton
              size="xs"
              color="primary"
              variant="soft"
              :disabled="!p.enabled || !canExecute"
              @click="selectCurrentPolicy(p)"
            >
              {{ isCurrentPolicy(p) ? "已选中" : "设为当前策略" }}
            </UButton>
          </div>
        </div>
        <p v-if="policies.length === 0" class="text-xs text-gray-500">暂无策略</p>
        <p v-else-if="!selectedPolicyExplicit" class="text-xs text-amber-600">请选择一个“当前执行策略”后再进入下一步。</p>
      </div>
    </UCard>

    <div v-else-if="currentStep === 1" class="grid gap-4 lg:grid-cols-3">
      <UCard class="lg:col-span-2">
        <template #header>
          <h3 class="text-sm font-semibold text-gray-800">Step 2：执行动作（策略：{{ selectedPolicy?.name || "-" }}）</h3>
        </template>
        <div class="space-y-2 text-sm">
          <p class="text-xs text-gray-500">动作将只针对当前执行策略，不会批量执行所有策略。</p>
          <UButton class="w-full" color="primary" :disabled="!canExecute || !selectedPolicyId" @click="runBackup">手动触发备份</UButton>
          <UButton class="w-full" color="neutral" variant="outline" :disabled="!canExecute" @click="runCleanup">触发清理</UButton>
          <p v-if="!selectedPolicyId" class="text-xs text-amber-600">请先选择启用中的策略。</p>
          <p class="text-xs text-gray-500">恢复验证请在 Step 3 的任务列表中对目标备份单条触发。</p>
        </div>
      </UCard>

      <UCard>
        <template #header>
          <h3 class="text-sm font-semibold text-gray-800">当前策略</h3>
        </template>
        <div class="space-y-1 text-sm">
          <p class="font-medium">{{ selectedPolicy?.name || "未选择" }}</p>
          <p class="text-xs text-gray-500">间隔：{{ selectedPolicy ? formatPolicyInterval(selectedPolicy) : "-" }}</p>
          <p class="text-xs text-gray-500">保留：{{ selectedPolicy?.retention_count || "-" }} 份</p>
          <p class="text-xs text-gray-500">时区：{{ selectedPolicy?.timezone || "-" }}</p>
          <p class="text-xs text-gray-500">目标：{{ selectedPolicy?.target_ref || "-" }}</p>
          <UButton class="mt-2 w-full" color="neutral" variant="outline" @click="goStep(0)">返回策略列表</UButton>
        </div>
      </UCard>
    </div>

    <UCard v-else>
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-gray-800">Step 3：观察结果（作业 / 告警 / 恢复任务）</h3>
          <UButton color="neutral" size="xs" variant="outline" @click="showAllPoliciesInStep3 = !showAllPoliciesInStep3">
            {{ showAllPoliciesInStep3 ? "仅看当前策略" : "查看全部策略" }}
          </UButton>
        </div>
        <p class="mt-1 text-xs text-gray-500">
          {{ showAllPoliciesInStep3 ? "当前显示全部策略数据" : `当前仅显示策略：${selectedPolicy?.name || "-"}` }}
        </p>
      </template>

      <BackupJobTable :items="displayedJobs" @restore-verify="runDrillForJob" />
      <div class="mt-3 flex flex-wrap items-center justify-between gap-2 text-xs text-gray-500">
        <div class="flex items-center gap-2">
          <span>任务总数：{{ jobsTotal }}</span>
          <span>第 {{ jobsPage }} / {{ jobsTotalPages }} 页</span>
          <select v-model.number="jobsPageSize" class="rounded border border-gray-300 px-2 py-1 text-xs" @change="reloadJobsFromFirstPage">
            <option :value="10">10 / 页</option>
            <option :value="20">20 / 页</option>
            <option :value="50">50 / 页</option>
          </select>
        </div>
        <div class="flex items-center gap-2">
          <UButton size="xs" color="neutral" variant="outline" :disabled="jobsPage <= 1" @click="gotoJobsPage(jobsPage - 1)">上一页</UButton>
          <UButton size="xs" color="neutral" variant="outline" :disabled="jobsPage >= jobsTotalPages" @click="gotoJobsPage(jobsPage + 1)">下一页</UButton>
        </div>
      </div>

      <div class="mt-4">
        <RestoreDrillPanel :drill="displayedLatestDrill" :history="displayedDrillHistory" :alerts="displayedAlerts" :streaming="drillStreaming" />
      </div>

      <div class="mt-4">
        <LogObservabilityPanel />
      </div>
    </UCard>

    <UCard>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="text-xs text-gray-500">{{ stepHint }}</p>
        <div class="flex gap-2">
          <UButton color="neutral" variant="outline" :disabled="currentStep === 0" @click="goPrevStep">
            {{ currentStep === 1 ? "返回策略选择" : "上一步" }}
          </UButton>
          <UButton color="primary" :disabled="!canGoNext" @click="goNextStep">
            {{ currentStep === 0 ? "进入执行" : currentStep === 1 ? "进入观察" : "留在当前步骤" }}
          </UButton>
        </div>
      </div>
    </UCard>

    <UModal v-model:open="policyModalOpen" :title="editingPolicyId ? '编辑策略' : '新建策略'" :ui="{ content: 'max-w-xl' }">
      <template #body>
        <div class="space-y-4">
          <div class="space-y-1">
            <label class="text-sm font-medium text-gray-800">策略名称</label>
            <input
              v-model="policyForm.name"
              class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
              placeholder="例如：生产库每日备份"
            />
          </div>

          <div class="grid gap-3 md:grid-cols-2">
            <div class="space-y-1">
              <label class="text-sm font-medium text-gray-800">执行间隔</label>
              <div class="flex gap-2">
                <input
                  v-model.number="policyForm.intervalValue"
                  type="number"
                  min="1"
                  class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
                />
                <select v-model="policyForm.intervalUnit" class="w-32 rounded border border-gray-300 px-3 py-2 text-sm">
                  <option value="minute">分钟</option>
                  <option value="hour">小时</option>
                  <option value="day">天</option>
                </select>
              </div>
              <p class="text-xs text-gray-500">支持分钟 / 小时 / 天，例如 15 分钟、6 小时、1 天。生产环境最小 1 小时。</p>
            </div>
            <div class="space-y-1">
              <label class="text-sm font-medium text-gray-800">保留份数</label>
              <input
                v-model.number="policyForm.retentionCount"
                type="number"
                min="1"
                class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
              />
              <p class="text-xs text-gray-500">超过数量后自动清理最旧备份。</p>
            </div>
          </div>

          <div class="grid gap-3 md:grid-cols-2">
            <div class="space-y-1">
              <label class="text-sm font-medium text-gray-800">时区</label>
              <input
                v-model="policyForm.timezone"
                class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
                placeholder="Asia/Shanghai"
              />
              <p class="text-xs text-gray-500">使用 IANA 时区标识。</p>
            </div>
            <div class="space-y-1">
              <label class="text-sm font-medium text-gray-800">目标库 / 实例</label>
              <input
                v-model="policyForm.targetRef"
                class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
                placeholder="powerx_bak"
              />
              <p class="text-xs text-gray-500">可填逻辑标识；如果留空会使用下方连接信息自动生成。</p>
            </div>
          </div>

          <div class="rounded-lg border border-gray-200 p-3">
            <h4 class="text-sm font-medium text-gray-800">目标连接测试（PostgreSQL）</h4>
            <p class="mt-1 text-xs text-gray-500">密码仅用于测试连接，不会写入策略存储。不填写时默认：127.0.0.1:5432 / postgres / powerx_bak。</p>
            <div class="mt-3 grid gap-3 md:grid-cols-2">
              <div class="space-y-1">
                <label class="text-sm font-medium text-gray-800">主机地址</label>
                <input v-model="policyForm.targetHost" class="w-full rounded border border-gray-300 px-3 py-2 text-sm" placeholder="127.0.0.1" />
              </div>
              <div class="space-y-1">
                <label class="text-sm font-medium text-gray-800">端口</label>
                <input v-model.number="policyForm.targetPort" type="number" min="1" max="65535" class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
              </div>
              <div class="space-y-1">
                <label class="text-sm font-medium text-gray-800">数据库名</label>
                <input v-model="policyForm.targetDatabase" class="w-full rounded border border-gray-300 px-3 py-2 text-sm" placeholder="powerx_bak" />
              </div>
              <div class="space-y-1">
                <label class="text-sm font-medium text-gray-800">用户名</label>
                <input v-model="policyForm.targetUsername" class="w-full rounded border border-gray-300 px-3 py-2 text-sm" placeholder="postgres" />
              </div>
              <div class="space-y-1">
                <label class="text-sm font-medium text-gray-800">密码</label>
                <input v-model="policyForm.targetPassword" type="password" class="w-full rounded border border-gray-300 px-3 py-2 text-sm" placeholder="请输入密码" />
              </div>
              <div class="space-y-1">
                <label class="text-sm font-medium text-gray-800">SSL 模式</label>
                <select v-model="policyForm.targetSSLMode" class="w-full rounded border border-gray-300 px-3 py-2 text-sm">
                  <option value="disable">disable</option>
                  <option value="require">require</option>
                  <option value="verify-ca">verify-ca</option>
                  <option value="verify-full">verify-full</option>
                </select>
              </div>
            </div>
            <div class="mt-3 flex items-center gap-2">
              <UButton
                color="neutral"
                variant="outline"
                :loading="testingTargetConnection"
                :disabled="testingTargetConnection"
                @click="testTargetConnection"
              >
                测试连接
              </UButton>
              <p
                v-if="targetConnectionResult"
                class="text-xs"
                :class="targetConnectionResult.ok ? 'text-emerald-600' : 'text-red-600'"
              >
                {{ targetConnectionResult.message }}
                <template v-if="targetConnectionResult.ok && targetConnectionResult.latencyMs !== undefined">
                  （{{ targetConnectionResult.latencyMs }} ms）
                </template>
              </p>
            </div>
            <p v-if="targetConnectionResult?.ok && targetConnectionResult.serverInfo" class="mt-1 text-xs text-gray-500">
              {{ targetConnectionResult.serverInfo }}
            </p>
          </div>

          <div class="rounded-lg border border-gray-200 p-3">
            <label class="inline-flex items-center gap-2 text-sm font-medium text-gray-800">
              <input v-model="policyForm.drillEnabled" type="checkbox" />
              启用恢复数据任务
            </label>
            <p class="mt-1 text-xs text-gray-500">开启后会按周期执行恢复数据任务。</p>
            <div v-if="policyForm.drillEnabled" class="mt-2 space-y-1">
              <label class="text-sm font-medium text-gray-800">恢复任务周期（天）</label>
              <input
                v-model.number="policyForm.drillIntervalDays"
                type="number"
                min="1"
                class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
              />
            </div>
          </div>

          <p v-if="policyError" class="text-xs text-red-600">{{ policyError }}</p>
        </div>
      </template>
      <template #footer>
        <div class="flex w-full justify-end gap-2">
          <UButton color="neutral" variant="ghost" @click="closePolicyModal">取消</UButton>
          <UButton color="primary" :disabled="!canExecute" @click="savePolicy">{{ editingPolicyId ? '更新策略' : '创建策略' }}</UButton>
        </div>
      </template>
    </UModal>
  </section>
</template>

<script setup lang="ts">
import type { StepperItem } from "@nuxt/ui";
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { useBackupOpsService, type BackupAlert, type BackupJob, type BackupPolicy, type RestoreDrillRecord } from "~/composables/api/services/backupOpsService";
import BackupJobTable from "~/components/ops/backup/BackupJobTable.vue";
import RestoreDrillPanel from "~/components/ops/backup/RestoreDrillPanel.vue";
import LogObservabilityPanel from "~/components/ops/backup/LogObservabilityPanel.vue";
import { useOpsAccess } from "~/composables/useOpsAccess";
import { useWSBus } from "~/composables/useWSBus";

type IntervalUnit = "minute" | "hour" | "day";

const backupSvc = useBackupOpsService();
const { canExecute, permissionHint, loadUserContext } = useOpsAccess();
const policies = ref<BackupPolicy[]>([]);
const jobs = ref<BackupJob[]>([]);
const jobsPage = ref(1);
const jobsPageSize = ref(20);
const jobsTotal = ref(0);
const alerts = ref<BackupAlert[]>([]);
const latestDrill = ref<RestoreDrillRecord | null>(null);
const drillHistory = ref<RestoreDrillRecord[]>([]);
const selectedPolicyId = ref("");
const selectedPolicyExplicit = ref(false);
const showAllPoliciesInStep3 = ref(false);
const editingPolicyId = ref<string>("");
const policyError = ref("");
const policyModalOpen = ref(false);

const filters = reactive({
  status: "" as "" | "enabled" | "disabled",
  keyword: "",
});

const alertFilter = reactive({
  level: "" as "" | "low" | "medium" | "high",
  acked: "",
});

const policyForm = reactive({
  name: "",
  intervalValue: 6,
  intervalUnit: "hour" as IntervalUnit,
  retentionCount: 14,
  timezone: "Asia/Shanghai",
  drillEnabled: true,
  drillIntervalDays: 7,
  targetRef: "powerx_bak",
  targetDriver: "postgres" as "postgres",
  targetHost: "",
  targetPort: 5432,
  targetDatabase: "",
  targetUsername: "",
  targetPassword: "",
  targetSSLMode: "disable" as "disable" | "require" | "verify-ca" | "verify-full",
  targetConnectTimeoutSec: 5,
});
const testingTargetConnection = ref(false);
const targetConnectionResult = ref<{ ok: boolean; message: string; latencyMs?: number; serverInfo?: string } | null>(null);
const lastTargetTestFingerprint = ref("");
const initialTargetFingerprint = ref("");

const selectedPolicyJobs = computed(() => jobs.value.filter((j) => String(j.policy_id) === selectedPolicyId.value));
const latestJob = computed(() => (selectedPolicyJobs.value.length > 0 ? selectedPolicyJobs.value[0] : null));
const enabledPolicies = computed(() => policies.value.filter((p) => !!p.enabled));
const selectedPolicy = computed(() => policies.value.find((p) => String(p.id) === selectedPolicyId.value) || null);
const hasTargetConnectionInput = computed(() => {
  return !!(
    policyForm.targetHost.trim() ||
    policyForm.targetDatabase.trim() ||
    policyForm.targetUsername.trim() ||
    policyForm.targetPassword.trim()
  );
});

const currentStep = ref(0);
const stepperItems = computed<StepperItem[]>(() => [
  { title: "配置策略", description: "创建并启用策略", icon: "i-lucide-settings-2" },
  { title: "执行动作", description: "触发备份/清理/恢复", icon: "i-lucide-play" },
  { title: "观察结果", description: "查看作业与告警", icon: "i-lucide-line-chart" },
]);

const maxAccessibleStep = computed(() => {
  if (enabledPolicies.value.length === 0 || !selectedPolicyExplicit.value) return 0;
  if (!selectedPolicy.value || !selectedPolicy.value.enabled) return 0;
  if (!latestJob.value) return 1;
  return 2;
});

const canGoNext = computed(() => currentStep.value < maxAccessibleStep.value);
const jobsTotalPages = computed(() => Math.max(1, Math.ceil(jobsTotal.value / Math.max(1, jobsPageSize.value))));

const stepHint = computed(() => {
  if (currentStep.value === 0) return selectedPolicyExplicit.value ? `当前执行策略：${selectedPolicy.value?.name || "-"}` : "先选择一个当前执行策略。";
  if (currentStep.value === 1) return `你正在执行策略：${selectedPolicy.value?.name || "-"}。`;
  return showAllPoliciesInStep3.value ? "当前正在查看全部策略数据。" : `当前正在查看策略：${selectedPolicy.value?.name || "-"}。`;
});

const displayedJobs = computed(() => jobs.value);
const displayedAlerts = computed(() => {
  if (showAllPoliciesInStep3.value) return alerts.value;
  return alerts.value.filter((a) => String(a.policy_id) === selectedPolicyId.value);
});
const displayedDrillHistory = computed(() => {
  if (showAllPoliciesInStep3.value) return drillHistory.value;
  const jobIds = new Set(selectedPolicyJobs.value.map((j) => String(j.id)));
  return drillHistory.value.filter((d) => jobIds.has(String(d.source_job_id)));
});
const displayedLatestDrill = computed(() => (displayedDrillHistory.value.length > 0 ? displayedDrillHistory.value[0] : null));

const isCurrentPolicy = (policy: BackupPolicy) => selectedPolicyExplicit.value && selectedPolicyId.value === String(policy.id);
const selectCurrentPolicy = async (policy: BackupPolicy) => {
  if (!policy.enabled) return;
  await backupSvc.setCurrentPolicy(policy.id);
  await loadPolicies();
  showAllPoliciesInStep3.value = false;
};

const goStep = (step: number) => {
  currentStep.value = Math.max(0, Math.min(step, maxAccessibleStep.value));
};

const goPrevStep = () => {
  goStep(currentStep.value - 1);
};

const goNextStep = () => {
  goStep(currentStep.value + 1);
};

watch(maxAccessibleStep, (limit) => {
  if (currentStep.value > limit) currentStep.value = limit;
}, { immediate: true });

const wsBus = useWSBus();
const drillStreaming = computed(() => wsBus.connected.value);
let wsDispose: (() => void) | null = null;
let backupWsDispose: (() => void) | null = null;

const isValidTimezone = (timezone: string): boolean => {
  try {
    Intl.DateTimeFormat("zh-CN", { timeZone: timezone });
    return true;
  } catch {
    return false;
  }
};

const parsePolicyInterval = (policy: BackupPolicy): { value: number; unit: IntervalUnit } => {
  const scheduleRaw = String(policy.schedule || "").trim().toLowerCase();
  const match = scheduleRaw.match(/^(\d+)\s*([a-z]+)$/);
  if (match) {
    const value = Number.parseInt(match[1] || "0", 10);
    const unitRaw = match[2];
    if (Number.isFinite(value) && value > 0) {
      if (["m", "min", "mins", "minute", "minutes"].includes(unitRaw)) return { value, unit: "minute" };
      if (["h", "hr", "hrs", "hour", "hours"].includes(unitRaw)) return { value, unit: "hour" };
      if (["d", "day", "days"].includes(unitRaw)) return { value, unit: "day" };
    }
  }
  return { value: Math.max(1, Number(policy.interval_hours || 6)), unit: "hour" };
};

const formatPolicyInterval = (policy: BackupPolicy): string => {
  const interval = parsePolicyInterval(policy);
  const unitLabel = interval.unit === "minute" ? "分钟" : interval.unit === "hour" ? "小时" : "天";
  return `${interval.value} ${unitLabel}`;
};

const validatePolicyForm = () => {
  if (!policyForm.name.trim()) return "策略名称不能为空";
  if (!Number.isFinite(policyForm.intervalValue) || policyForm.intervalValue <= 0) return "执行间隔必须大于 0";
  if (!["minute", "hour", "day"].includes(policyForm.intervalUnit)) return "执行间隔单位不合法";
  if (!Number.isFinite(policyForm.retentionCount) || policyForm.retentionCount <= 0) return "保留份数必须大于 0";
  const timezone = policyForm.timezone.trim();
  if (!timezone) return "时区不能为空";
  if (!isValidTimezone(timezone)) return "时区不合法，请使用 IANA 时区（例如 Asia/Shanghai）";
  if (!Number.isFinite(policyForm.drillIntervalDays) || policyForm.drillIntervalDays <= 0) return "恢复任务周期天数必须大于 0";
  return "";
};

const loadPolicies = async () => {
  const result = await backupSvc.listPolicies({ status: filters.status || undefined, keyword: filters.keyword || undefined, page: 1, pageSize: 50 });
  policies.value = result.items;
  const current = result.items.find((p) => !!p.enabled && !!p.is_current);
  if (current) {
    selectedPolicyId.value = String(current.id);
    selectedPolicyExplicit.value = true;
    return;
  }
  if (selectedPolicyId.value) {
    const exists = result.items.some((p) => String(p.id) === selectedPolicyId.value && !!p.enabled);
    if (!exists) {
      selectedPolicyId.value = "";
      selectedPolicyExplicit.value = false;
    }
  }
};

const loadJobs = async () => {
  const result = await backupSvc.listJobs({
    policyId: showAllPoliciesInStep3.value ? undefined : (selectedPolicyId.value || undefined),
    page: jobsPage.value,
    pageSize: jobsPageSize.value,
  });
  jobs.value = result.items;
  jobsTotal.value = result.total;
  if (jobsPage.value > jobsTotalPages.value) {
    jobsPage.value = jobsTotalPages.value;
  }
};

const reloadJobsFromFirstPage = async () => {
  jobsPage.value = 1;
  await loadJobs();
};

const gotoJobsPage = async (page: number) => {
  const next = Math.max(1, Math.min(page, jobsTotalPages.value));
  if (next === jobsPage.value) return;
  jobsPage.value = next;
  await loadJobs();
};

const loadAlerts = async () => {
  const acked = alertFilter.acked === "" ? undefined : alertFilter.acked === "true";
  const result = await backupSvc.listAlerts({ level: alertFilter.level || undefined, acked, page: 1, pageSize: 20 });
  alerts.value = result.items;
};

const load = async () => {
  await Promise.all([loadPolicies(), loadJobs(), loadAlerts()]);
  await loadDrillHistory();
};

const resetForm = () => {
  editingPolicyId.value = "";
  policyError.value = "";
  policyForm.name = "";
  policyForm.intervalValue = 6;
  policyForm.intervalUnit = "hour";
  policyForm.retentionCount = 14;
  policyForm.timezone = "Asia/Shanghai";
  policyForm.drillEnabled = true;
  policyForm.drillIntervalDays = 7;
  policyForm.targetRef = "powerx_bak";
  policyForm.targetDriver = "postgres";
  policyForm.targetHost = "";
  policyForm.targetPort = 5432;
  policyForm.targetDatabase = "";
  policyForm.targetUsername = "";
  policyForm.targetPassword = "";
  policyForm.targetSSLMode = "disable";
  policyForm.targetConnectTimeoutSec = 5;
  targetConnectionResult.value = null;
  lastTargetTestFingerprint.value = "";
  initialTargetFingerprint.value = buildTargetTestFingerprint();
};

const openCreatePolicyModal = () => {
  resetForm();
  policyModalOpen.value = true;
};

const openEditPolicyModal = (p: BackupPolicy) => {
  resetForm();
  editingPolicyId.value = String(p.id);
  policyError.value = "";
  policyForm.name = p.name;
  const interval = parsePolicyInterval(p);
  policyForm.intervalValue = interval.value;
  policyForm.intervalUnit = interval.unit;
  policyForm.retentionCount = p.retention_count || 14;
  policyForm.timezone = p.timezone || "Asia/Shanghai";
  policyForm.drillEnabled = !!p.drill_enabled;
  policyForm.drillIntervalDays = p.drill_interval_days || 7;
  policyForm.targetRef = p.target_ref || "powerx_bak";
  targetConnectionResult.value = null;
  lastTargetTestFingerprint.value = "";
  initialTargetFingerprint.value = buildTargetTestFingerprint();
  policyModalOpen.value = true;
};

const closePolicyModal = () => {
  policyModalOpen.value = false;
  resetForm();
};

const composeTargetRefFromConnection = () => {
  const host = policyForm.targetHost.trim();
  const database = policyForm.targetDatabase.trim();
  const username = policyForm.targetUsername.trim();
  if (!host || !database || !username) return "";
  return `${policyForm.targetDriver}://${username}@${host}:${policyForm.targetPort}/${database}`;
};

const testTargetConnection = async () => {
  if (testingTargetConnection.value) return;
  targetConnectionResult.value = null;
  const host = policyForm.targetHost.trim() || "127.0.0.1";
  const database = policyForm.targetDatabase.trim() || policyForm.targetRef.trim() || "powerx_bak";
  const username = policyForm.targetUsername.trim() || "postgres";
  try {
    testingTargetConnection.value = true;
    const resp = await backupSvc.testTargetConnection({
      driver: policyForm.targetDriver,
      host,
      port: policyForm.targetPort,
      database,
      username,
      password: policyForm.targetPassword,
      ssl_mode: policyForm.targetSSLMode,
      connect_timeout_sec: policyForm.targetConnectTimeoutSec,
    });
    targetConnectionResult.value = {
      ok: !!resp.reachable,
      message: resp.message || "连接成功",
      latencyMs: resp.latency_ms,
      serverInfo: resp.server_info,
    };
    policyError.value = "";
    lastTargetTestFingerprint.value = buildTargetTestFingerprint();
  } catch (error: any) {
    targetConnectionResult.value = { ok: false, message: error?.message || "连接测试失败" };
    lastTargetTestFingerprint.value = "";
  } finally {
    testingTargetConnection.value = false;
  }
};

const buildTargetTestFingerprint = () => {
  return [
    policyForm.targetDriver,
    policyForm.targetHost.trim(),
    String(policyForm.targetPort),
    policyForm.targetDatabase.trim(),
    policyForm.targetUsername.trim(),
    policyForm.targetPassword,
    policyForm.targetSSLMode,
    String(policyForm.targetConnectTimeoutSec),
  ].join("|");
};

const savePolicy = async () => {
  if (!canExecute.value) return;
  policyError.value = validatePolicyForm();
  if (policyError.value) return;
  if (!policyForm.targetRef.trim() && hasTargetConnectionInput.value) {
    const currentFingerprint = buildTargetTestFingerprint();
    const connectionChanged = currentFingerprint !== initialTargetFingerprint.value;
    if (connectionChanged && (!targetConnectionResult.value?.ok || lastTargetTestFingerprint.value !== currentFingerprint)) {
      policyError.value = "请先对当前连接参数执行“测试连接”并确保通过，再保存策略";
      return;
    }
  }

  const payload = {
    name: policyForm.name.trim(),
    interval_value: policyForm.intervalValue,
    interval_unit: policyForm.intervalUnit,
    interval_hours: policyForm.intervalUnit === "hour" ? policyForm.intervalValue : undefined,
    retention_count: policyForm.retentionCount,
    timezone: policyForm.timezone.trim(),
    drill_enabled: policyForm.drillEnabled,
    drill_interval_days: policyForm.drillIntervalDays,
    target_ref: policyForm.targetRef.trim() || composeTargetRefFromConnection() || "powerx_bak",
  };

  try {
    if (editingPolicyId.value) {
      await backupSvc.updatePolicy(editingPolicyId.value, payload);
    } else {
      await backupSvc.createPolicy(payload);
    }
    await loadPolicies();
    closePolicyModal();
  } catch (error: any) {
    policyError.value = error?.message || "策略保存失败，请稍后重试";
  }
};

const togglePolicy = async (p: BackupPolicy, enabled: boolean) => {
  if (!canExecute.value) return;
  if (enabled) await backupSvc.enablePolicy(p.id);
  else await backupSvc.disablePolicy(p.id);
  await loadPolicies();
  if (!enabled && selectedPolicyId.value === String(p.id)) {
    selectedPolicyId.value = "";
    selectedPolicyExplicit.value = false;
    goStep(0);
  }
};

const runBackup = async () => {
  if (!canExecute.value || !selectedPolicyId.value) return;
  await backupSvc.triggerJob(selectedPolicyId.value);
  jobsPage.value = 1;
  await loadJobs();
  goStep(2);
};

const runCleanup = async () => {
  if (!canExecute.value) return;
  await backupSvc.triggerCleanup();
  await loadAlerts();
};

const runDrillForJob = async (job: BackupJob) => {
  if (!canExecute.value || !job?.id) return;
  latestDrill.value = await backupSvc.createRestoreDrill({ source_job_id: job.id, reason: "manual_restore_verify" });
  await loadDrillHistory();
};

const ackAlert = async (alertId: string | number) => {
  if (!canExecute.value) return;
  await backupSvc.ackAlert(alertId);
  await loadAlerts();
};

const loadDrillHistory = async () => {
  const result = await backupSvc.listRestoreDrills({ page: 1, pageSize: 10 });
  drillHistory.value = result.items;
  latestDrill.value = result.items.length > 0 ? result.items[0] : latestDrill.value;
};

const subscribeDrillEvents = async () => {
  try {
    await wsBus.connect();
    wsDispose = wsBus.subscribe("_topic.ops.backup.restore_drill.status", async () => {
      await loadDrillHistory();
    });
    backupWsDispose = wsBus.subscribe("_topic.system.notification", async (payload: any) => {
      const kind = String(payload?.kind || "").trim().toLowerCase();
      if (kind !== "ops.backup.job" && kind !== "ops.backup.restore") {
        return;
      }
      await Promise.all([loadJobs(), loadDrillHistory()]);
    });
  } catch {
    wsDispose = null;
    backupWsDispose = null;
  }
};

onMounted(async () => {
  await loadUserContext();
  await load();
  await subscribeDrillEvents();
});

onBeforeUnmount(() => {
  if (wsDispose) wsDispose();
  wsDispose = null;
  if (backupWsDispose) backupWsDispose();
  backupWsDispose = null;
});

watch([showAllPoliciesInStep3, selectedPolicyId], async () => {
  jobsPage.value = 1;
  await loadJobs();
});

watch(
  () => [
    policyForm.name,
    policyForm.intervalValue,
    policyForm.intervalUnit,
    policyForm.retentionCount,
    policyForm.timezone,
    policyForm.drillEnabled,
    policyForm.drillIntervalDays,
    policyForm.targetRef,
  ],
  () => {
    policyError.value = "";
  },
);

watch(
  () => [
    policyForm.targetDriver,
    policyForm.targetHost,
    policyForm.targetPort,
    policyForm.targetDatabase,
    policyForm.targetUsername,
    policyForm.targetPassword,
    policyForm.targetSSLMode,
    policyForm.targetConnectTimeoutSec,
  ],
  () => {
    policyError.value = "";
    targetConnectionResult.value = null;
    lastTargetTestFingerprint.value = "";
  },
);
</script>
