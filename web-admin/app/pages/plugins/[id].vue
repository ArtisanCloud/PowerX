<template>
  <div class="space-y-6 p-4 sm:p-6">
    <div class="flex items-center justify-between">
      <div class="flex items-start gap-3">
        <div
          class="w-12 h-12 rounded-md bg-[var(--ui-bg-elevated)] border border-[var(--border-color)] flex items-center justify-center shrink-0 overflow-hidden"
        >
          <img
            v-if="plugin?.icon && !imageError"
            :src="plugin?.icon"
            alt=""
            class="w-12 h-12 object-cover"
            @error="handleImageError"
          />
          <UIcon
            v-else
            name="i-heroicons-puzzle-piece-20-solid"
            class="w-7 h-7 text-[var(--text-secondary)]"
          />
        </div>
        <div>
          <div class="text-xl font-semibold text-[var(--text-primary)]">
            {{ plugin?.name || id }}
          </div>
          <div class="text-sm text-[var(--text-secondary)]">
            版本 {{ plugin?.version || "-" }} · 作者
            {{ plugin?.author || "-" }} · 分类 {{ plugin?.category || "-" }}
          </div>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <UButton
          size="sm"
          variant="ghost"
          icon="i-heroicons-arrow-left"
          :to="'/plugins/market'"
          >返回</UButton
        >
        <UButton
          v-if="isRoot && !sysInstalled"
          size="sm"
          color="primary"
          icon="i-heroicons-arrow-down-tray"
          @click="installOpen = true"
          >安装</UButton
        >
        <UButton
          v-if="isRoot && sysInstalled"
          size="sm"
          variant="outline"
          :color="topUninstallAction.color"
          :icon="topUninstallAction.icon"
          :disabled="topUninstallAction.disabled"
          @click="handleUninstallAction"
          >{{ topUninstallAction.label }}</UButton
        >
        <!-- 顶部不放启用/停用与刷新，避免与下方系统卡片重复 -->
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <div class="lg:col-span-2 space-y-6">
        <!-- 介绍 -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-2">介绍</div>
          <p class="text-sm text-[var(--text-secondary)] whitespace-pre-wrap">
            {{ plugin?.description || "-" }}
          </p>
        </div>

        <!-- 版本变更/更新日志（示例） -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-2">
            版本与更新
          </div>
          <ul
            class="list-disc list-inside text-sm text-[var(--text-secondary)] space-y-1"
          >
            <li>v{{ plugin?.version }} 修复若干问题，提升稳定性</li>
            <li>支持更多平台与模型适配</li>
            <li>优化文档与示例</li>
          </ul>
        </div>

        <!-- 权限声明（示例） -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-2">权限</div>
          <div class="text-sm text-[var(--text-secondary)] space-y-1">
            <div>• 网络访问（请求外部 API）</div>
            <div>• 本地存储（读写插件数据）</div>
            <div>• 文件访问（读取/上传文件）</div>
          </div>
        </div>
      </div>

      <div class="space-y-6">
        <!-- 侧边信息 -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-3">
            统计与标签
          </div>
          <div class="text-sm text-[var(--text-secondary)]">
            {{ isRoot ? "安装量" : "订阅量" }}：{{ formatCount(plugin?.installs || 0) }}
          </div>
          <div class="mt-2 flex flex-wrap gap-2">
            <UBadge
              v-for="t in plugin?.tags || []"
              :key="t"
              variant="soft"
              size="xs"
              >{{ t }}</UBadge
            >
          </div>
        </div>

        <!-- 系统控制 -->
        <div
          v-if="isRoot"
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-3">
            系统运行
          </div>
          <div class="text-sm text-[var(--text-secondary)] space-y-2">
            <div>
              系统启用：<UBadge
                :color="sysEnabled ? 'green' : 'neutral'"
                size="xs"
                >{{ sysEnabled ? "是" : "否" }}</UBadge
              >
            </div>
            <div>状态：{{ sysStatus || "-" }}</div>
            <div>
              卸载状态：<UBadge :color="drainBadge.color" size="xs">{{
                drainBadge.label
              }}</UBadge>
            </div>
            <div
              v-if="latestDrainJob"
              class="rounded-md border border-[var(--border-color)] bg-[var(--ui-bg-elevated)] p-2 text-xs space-y-1"
            >
              <div>drain_job：{{ latestDrainJob.job_id || latestDrainJob.id || "-" }}</div>
              <div>状态：{{ latestDrainJob.status || "-" }}</div>
              <div v-if="drainBlockerSummary">
                阻断：{{ drainBlockerSummary }}
              </div>
            </div>
            <div class="flex flex-wrap items-center gap-2 mt-2">
              <UButton
                v-if="isRoot"
                size="sm"
                :variant="sysEnabled ? 'outline' : 'solid'"
                :color="sysEnabled ? 'error' : 'primary'"
                :icon="sysEnabled ? 'i-heroicons-pause' : 'i-heroicons-play'"
                :disabled="!canToggleSystem"
                @click="toggleEnable"
              >
                {{ sysEnabled ? "停用" : "启用" }}
              </UButton>
              <UButton
                v-if="isRoot && sysInstalled"
                size="sm"
                variant="outline"
                icon="i-heroicons-arrow-path"
                :disabled="!canMutateSystemRuntime"
                @click="restartPlugin"
                >重启</UButton
              >
              <UButton
                v-if="isRoot && sysInstalled"
                size="sm"
                variant="outline"
                icon="i-heroicons-arrow-up-on-square"
                :disabled="!canMutateSystemRuntime"
                @click="switchVersion"
                >切换版本</UButton
              >
              <UButton
                size="sm"
                variant="ghost"
                icon="i-heroicons-clipboard-document-list"
                @click="openLogs"
                >查看日志</UButton
              >
              <UButton
                size="sm"
                variant="ghost"
                icon="i-heroicons-arrow-path"
                @click="refreshPluginLifecycleState"
                >刷新状态</UButton
              >
              <UButton
                v-if="isRoot && isDrainActive"
                size="sm"
                variant="ghost"
                icon="i-heroicons-list-bullet"
                @click="openDrainBlockers"
                >查看阻断详情</UButton
              >
              <UButton
                v-if="isRoot && isDrainActive"
                size="sm"
                variant="outline"
                color="warning"
                icon="i-heroicons-x-circle"
                :disabled="!hasDrainBlockers"
                @click="openDrainBlockers"
                >选择要取消的任务</UButton
              >
            </div>
          </div>
        </div>

        <!-- 租户控制（root 或当前租户管理员可见） -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-3">本租户</div>
          <div class="text-sm text-[var(--text-secondary)] space-y-2">
            <div>
              订阅：
              <UBadge :color="tenantEnabled ? 'green' : 'neutral'" size="xs">{{
                tenantEnabled ? "已订阅" : "未订阅"
              }}</UBadge>
            </div>
            <div>
              租户实例状态：<UBadge :color="tenantStatusBadge.color" size="xs">{{
                tenantStatusBadge.label
              }}</UBadge>
            </div>
            <div
              v-if="isDrained"
              class="rounded-md border border-[var(--border-color)] bg-[var(--ui-bg-elevated)] p-2 text-xs text-[var(--text-secondary)]"
            >
              所有租户实例已退出，root 可以执行最终卸载。
            </div>
            <div v-if="clientId">
              client_id：<code>{{ clientId }}</code>
            </div>
            <div class="flex flex-wrap items-center gap-2 mt-2">
              <UButton
                v-if="canManageTenantInstance && !isDrained"
                size="sm"
                :variant="tenantEnabled ? 'outline' : 'solid'"
                :color="tenantEnabled ? 'error' : 'primary'"
                :icon="tenantEnabled ? 'i-heroicons-pause' : 'i-heroicons-play'"
                :disabled="!canToggleTenant"
                @click="toggleTenant"
              >
                {{ tenantEnabled ? "取消订阅" : "订阅启用" }}
              </UButton>
              <UButton
                v-if="canManageTenantInstance && tenantEnabled"
                size="sm"
                variant="outline"
                icon="i-heroicons-key"
                @click="rotateTenantSecret"
                >轮换凭证</UButton
              >
              <UButton
                v-if="canManageTenantInstance"
                size="sm"
                variant="outline"
                color="error"
                icon="i-heroicons-trash"
                @click="deleteTenantConfig"
                >删除订阅配置</UButton
              >
              <UButton
                size="sm"
                variant="ghost"
                icon="i-heroicons-arrow-path"
                @click="refreshPluginLifecycleState"
                >刷新</UButton
              >
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 安装对话框 -->
    <InstallDialog
      :model-value="installOpen"
      @update:modelValue="(v) => (installOpen.value = v)"
      :plugin="plugin"
      @installed="onInstalled"
    />

    <UModal
      v-model:open="drainBlockersOpen"
      title="Drain 阻断详情"
      description="这些未完成任务会阻止插件进入可最终卸载状态。"
      :ui="{ content: 'max-w-4xl w-full' }"
    >
      <template #body>
        <div class="space-y-4">
          <div class="rounded-md border border-amber-400/40 bg-amber-500/10 p-3 text-sm text-amber-100">
            <div class="font-medium">处理建议</div>
            <div class="mt-1">{{ drainBlockerRecommendation }}</div>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div class="rounded-md border border-[var(--border-color)] bg-[var(--ui-bg-elevated)] p-3">
              <div class="text-xs text-[var(--text-secondary)]">Event Tasks</div>
              <div class="text-2xl font-semibold text-[var(--text-primary)]">{{ drainBlockerEventTaskCount }}</div>
            </div>
            <div class="rounded-md border border-[var(--border-color)] bg-[var(--ui-bg-elevated)] p-3">
              <div class="text-xs text-[var(--text-secondary)]">Scheduler Jobs</div>
              <div class="text-2xl font-semibold text-[var(--text-primary)]">{{ drainBlockerSchedulerJobCount }}</div>
            </div>
          </div>

          <div
            v-if="!hasDrainBlockers"
            class="rounded-md border border-emerald-500/40 bg-emerald-500/10 p-3 text-sm text-emerald-900 dark:text-emerald-100"
          >
            当前没有检测到 Event Task 或 Scheduler Job 阻断。刷新状态后，插件应进入可最终卸载状态；如果仍停留在准备卸载中，说明租户实例状态没有被后端正确收敛。
          </div>

          <div v-if="drainBlockerEventTaskCount > 0" class="space-y-2">
            <div class="flex items-center justify-between gap-2">
              <div>
                <div class="font-medium text-[var(--text-primary)]">Event Task 阻断</div>
                <div class="text-xs text-[var(--text-secondary)]">
                  共 {{ eventTaskPagination.total }} 条，第 {{ eventTaskPagination.page }}/{{ eventTaskPageCount }} 页
                </div>
              </div>
              <div class="flex flex-wrap items-center gap-2">
                <UButton size="xs" variant="ghost" @click="selectCurrentDrainBlockerPage('event_task')">
                  全选当前页
                </UButton>
                <UButton size="xs" variant="ghost" :disabled="selectedDrainBlockerCount === 0" @click="clearSelectedDrainBlockers">
                  清空选择
                </UButton>
              </div>
            </div>
            <div class="overflow-x-auto rounded-md border border-[var(--border-color)]">
              <table class="min-w-full text-xs">
                <thead class="bg-[var(--ui-bg-elevated)] text-[var(--text-secondary)]">
                  <tr>
                    <th class="px-3 py-2 text-left">选择</th>
                    <th class="px-3 py-2 text-left">ID</th>
                    <th class="px-3 py-2 text-left">状态</th>
                    <th class="px-3 py-2 text-left">Subscriber</th>
                    <th class="px-3 py-2 text-left">Topic</th>
                    <th class="px-3 py-2 text-left">错误</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="task in drainBlockerEventTasks" :key="task.id || task.task_id" class="border-t border-[var(--border-color)]">
                    <td class="px-3 py-2">
                      <UCheckbox
                        :model-value="selectedDrainEventTaskIDs.has(Number(task.id))"
                        @update:modelValue="(checked) => toggleDrainEventTask(task.id, Boolean(checked))"
                      />
                    </td>
                    <td class="px-3 py-2 font-mono">{{ task.id || task.task_id }}</td>
                    <td class="px-3 py-2">{{ task.status || "-" }}</td>
                    <td class="px-3 py-2 font-mono">{{ task.subscriber_id || "-" }}</td>
                    <td class="px-3 py-2 font-mono">{{ task.topic || "-" }}</td>
                    <td class="px-3 py-2 max-w-md whitespace-normal">{{ task.error_message || "-" }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-if="eventTaskPagination.total > eventTaskPagination.pageSize" class="flex justify-end">
              <UPagination
                v-model:page="eventTaskPagination.page"
                :total="eventTaskPagination.total"
                :items-per-page="eventTaskPagination.pageSize"
                :sibling-count="1"
                show-edges
                @update:page="() => loadDrainBlockers('event_task')"
              />
            </div>
          </div>

          <div v-if="drainBlockerSchedulerJobCount > 0" class="space-y-2">
            <div class="flex items-center justify-between gap-2">
              <div>
                <div class="font-medium text-[var(--text-primary)]">Scheduler Job 阻断</div>
                <div class="text-xs text-[var(--text-secondary)]">
                  共 {{ schedulerJobPagination.total }} 条，第 {{ schedulerJobPagination.page }}/{{ schedulerJobPageCount }} 页
                </div>
              </div>
              <div class="flex flex-wrap items-center gap-2">
                <UButton size="xs" variant="ghost" @click="selectCurrentDrainBlockerPage('scheduler_job')">
                  全选当前页
                </UButton>
                <UButton size="xs" variant="ghost" :disabled="selectedDrainBlockerCount === 0" @click="clearSelectedDrainBlockers">
                  清空选择
                </UButton>
              </div>
            </div>
            <div class="overflow-x-auto rounded-md border border-[var(--border-color)]">
              <table class="min-w-full text-xs">
                <thead class="bg-[var(--ui-bg-elevated)] text-[var(--text-secondary)]">
                  <tr>
                    <th class="px-3 py-2 text-left">选择</th>
                    <th class="px-3 py-2 text-left">UUID</th>
                    <th class="px-3 py-2 text-left">名称</th>
                    <th class="px-3 py-2 text-left">状态</th>
                    <th class="px-3 py-2 text-left">下次执行</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="job in drainBlockerSchedulerJobs" :key="job.uuid || job.name" class="border-t border-[var(--border-color)]">
                    <td class="px-3 py-2">
                      <UCheckbox
                        :model-value="selectedDrainSchedulerJobUUIDs.has(String(job.uuid || ''))"
                        @update:modelValue="(checked) => toggleDrainSchedulerJob(job.uuid, Boolean(checked))"
                      />
                    </td>
                    <td class="px-3 py-2 font-mono">{{ job.uuid || "-" }}</td>
                    <td class="px-3 py-2">{{ job.name || "-" }}</td>
                    <td class="px-3 py-2">{{ job.status || "-" }}</td>
                    <td class="px-3 py-2">{{ job.next_run_at || "-" }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-if="schedulerJobPagination.total > schedulerJobPagination.pageSize" class="flex justify-end">
              <UPagination
                v-model:page="schedulerJobPagination.page"
                :total="schedulerJobPagination.total"
                :items-per-page="schedulerJobPagination.pageSize"
                :sibling-count="1"
                show-edges
                @update:page="() => loadDrainBlockers('scheduler_job')"
              />
            </div>
          </div>

          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="drainBlockersOpen = false">关闭</UButton>
            <UButton
              color="warning"
              icon="i-heroicons-x-circle"
              :disabled="selectedDrainBlockerCount === 0"
              @click="cancelDrainBlockers"
            >
              取消选中任务
            </UButton>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script lang="ts">
const pluginDetailMetaRefreshPromises = new Map<string, Promise<void>>();
const pluginDetailLifecycleRefreshPromises = new Map<string, Promise<void>>();
const pluginDetailLastLifecycleRefreshAt = new Map<string, number>();
</script>

<script setup lang="ts">
import InstallDialog from "~/components/plugins/InstallDialog.vue";
import type { MarketplacePlugin } from "~/components/plugins/PluginCard.vue";
import { useUserStore } from "~/stores/user";
import { useWSBus } from "~/composables/useWSBus";
import {
  LazyPluginsLogsModal,
  LazyPluginsSwitchVersionModal,
} from "#components";
import { useToast } from "#imports";

definePageMeta({
  layout: "default",
});

const route = useRoute();
const router = useRouter();
const id = computed(() => String(route.params.id || ""));
const plugin = ref<MarketplacePlugin | undefined>(undefined);
const imageError = ref(false);

const installOpen = ref(false);
const toast = useToast();
const wsBus = useWSBus();
const menuRefreshToken = useState<number>("px-menu-refresh-token", () => 0);

// 系统状态
const sysEnabled = ref<boolean>(false);
const sysInstalled = ref<boolean>(false);
const sysStatus = ref<string>("");
const currentVersion = ref<string>("");
const tenantEnabled = ref<boolean>(false);
const tenantStatus = ref<string>("");
const clientId = ref<string>("");
const drainJobs = ref<any[]>([]);

const showSecret = ref(false);
const oneTimeSecret = ref<string>("");
const pluginMissing = ref(false);
const drainBlockersOpen = ref(false);
const selectedDrainEventTaskIDs = ref<Set<number>>(new Set());
const selectedDrainSchedulerJobUUIDs = ref<Set<string>>(new Set());
const drainBlockerEventTaskRows = ref<any[]>([]);
const drainBlockerSchedulerJobRows = ref<any[]>([]);
const eventTaskPagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
});
const schedulerJobPagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
});

const DRAIN_ACTIVE_TENANT_STATUSES = new Set([
  "draining_requested",
  "disabled_by_platform",
]);
const DRAIN_ACTIVE_JOB_STATUSES = new Set([
  "requested",
  "draining",
  "blocking_new_usage",
  "waiting",
  "pending",
  "active",
]);
const DRAIN_READY_JOB_STATUSES = new Set([
  "ready_to_uninstall",
  "drained",
  "completed",
  "complete",
]);

const latestDrainJob = computed(() => drainJobs.value[0] || null);
const drainBlockerData = computed(() => {
  const raw = latestDrainJob.value?.last_blocker_json || latestDrainJob.value?.LastBlockerJSON;
  if (!raw) return null;
  return typeof raw === "string" ? safeParseJSON(raw) : raw;
});
const drainBlockerEventTasks = computed(() =>
  drainBlockerEventTaskRows.value.length > 0
    ? drainBlockerEventTaskRows.value
    : Array.isArray(drainBlockerData.value?.event_tasks) ? drainBlockerData.value.event_tasks : []
);
const drainBlockerSchedulerJobs = computed(() =>
  drainBlockerSchedulerJobRows.value.length > 0
    ? drainBlockerSchedulerJobRows.value
    : Array.isArray(drainBlockerData.value?.scheduler_jobs) ? drainBlockerData.value.scheduler_jobs : []
);
const drainBlockerEventTaskCount = computed(() =>
  Number(eventTaskPagination.total || drainBlockerData.value?.event_task_count || drainBlockerEventTasks.value.length || 0)
);
const drainBlockerSchedulerJobCount = computed(() =>
  Number(schedulerJobPagination.total || drainBlockerData.value?.scheduler_job_count || drainBlockerSchedulerJobs.value.length || 0)
);
const eventTaskPageCount = computed(() =>
  Math.max(1, Math.ceil(eventTaskPagination.total / eventTaskPagination.pageSize))
);
const schedulerJobPageCount = computed(() =>
  Math.max(1, Math.ceil(schedulerJobPagination.total / schedulerJobPagination.pageSize))
);
const hasDrainBlockers = computed(
  () => drainBlockerEventTaskCount.value > 0 || drainBlockerSchedulerJobCount.value > 0
);
const selectedDrainBlockerCount = computed(
  () => selectedDrainEventTaskIDs.value.size + selectedDrainSchedulerJobUUIDs.value.size
);
const drainBlockerSummary = computed(() => {
  if (!hasDrainBlockers.value) return "";
  const parts: string[] = [];
  if (drainBlockerEventTaskCount.value > 0) {
    parts.push(`${drainBlockerEventTaskCount.value} 个 Event Task`);
  }
  if (drainBlockerSchedulerJobCount.value > 0) {
    parts.push(`${drainBlockerSchedulerJobCount.value} 个 Scheduler Job`);
  }
  return parts.join("，");
});
const drainBlockerRecommendation = computed(() => {
  const firstEventTask = drainBlockerEventTasks.value[0];
  const message = String(firstEventTask?.error_message || "");
  if (message.includes("IndexedDB") || message.includes("后端无法读取")) {
    return "推荐取消：这类任务依赖浏览器本地素材，后端无法自动恢复。若你还需要导出结果，应先回到插件业务页面重新上传或关联素材；如果目标是卸载插件，可以取消未完成任务并继续卸载。";
  }
  if (drainBlockerEventTaskCount.value > 0 || drainBlockerSchedulerJobCount.value > 0) {
    return "如果这些任务还有业务价值，先回到对应插件处理完成；如果当前目标是下线或卸载插件，root 可以取消未完成任务并继续卸载。取消后任务不会再执行。";
  }
  return "当前没有检测到阻断任务。";
});
const normalizedTenantStatus = computed(() => (tenantStatus.value || "").toLowerCase());
const normalizedDrainJobStatus = computed(() =>
  String(latestDrainJob.value?.status || "").toLowerCase()
);
const isDrainActive = computed(() => {
  if (DRAIN_ACTIVE_TENANT_STATUSES.has(normalizedTenantStatus.value)) return true;
  if (DRAIN_ACTIVE_JOB_STATUSES.has(normalizedDrainJobStatus.value)) return true;
  return false;
});
const isDrained = computed(() => {
  if (normalizedTenantStatus.value === "drained") return true;
  if (DRAIN_READY_JOB_STATUSES.has(normalizedDrainJobStatus.value)) return true;
  return false;
});
const canStartDrain = computed(
  () => isRoot.value && sysInstalled.value && !isDrainActive.value && !isDrained.value
);
const canFinalUninstall = computed(
  () => isRoot.value && sysInstalled.value && isDrained.value
);
const canToggleSystem = computed(
  () => isRoot.value && sysInstalled.value && !isDrainActive.value && !isDrained.value
);
const canMutateSystemRuntime = computed(
  () => isRoot.value && sysInstalled.value && !isDrainActive.value && !isDrained.value
);
const canManageTenantInstance = computed(() => isRoot.value || isTenantAdmin.value);
const canToggleTenant = computed(() => {
  if (!canManageTenantInstance.value) return false;
  if (tenantEnabled.value) return true;
  return !isDrainActive.value && !isDrained.value;
});
const topUninstallAction = computed(() => {
  if (isDrained.value) {
    return {
      label: "最终卸载",
      color: "error" as const,
      icon: "i-heroicons-trash",
      disabled: !canFinalUninstall.value,
    };
  }
  if (isDrainActive.value) {
    return {
      label: "等待 drain 完成",
      color: "neutral" as const,
      icon: "i-heroicons-clock",
      disabled: true,
    };
  }
  return {
    label: "准备卸载",
    color: "warning" as const,
    icon: "i-heroicons-no-symbol",
    disabled: !canStartDrain.value,
  };
});
const drainBadge = computed(() => {
  if (isDrained.value) return { label: "可最终卸载", color: "success" as const };
  if (isDrainActive.value) return { label: "准备卸载中", color: "warning" as const };
  return { label: "未发起", color: "neutral" as const };
});
const effectiveTenantStatus = computed(() => {
  if (isDrained.value) return "drained";
  return tenantStatus.value || (tenantEnabled.value ? "enabled" : "none");
});
const tenantStatusBadge = computed(() => {
  const status = effectiveTenantStatus.value;
  if (status === "drained") return { label: "drained", color: "success" as const };
  if (DRAIN_ACTIVE_TENANT_STATUSES.has(status)) {
    return { label: status, color: "warning" as const };
  }
  return { label: status, color: tenantEnabled.value ? ("green" as const) : ("neutral" as const) };
});

function safeParseJSON(value: string) {
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}

function toggleDrainEventTask(taskID: any, checked: boolean) {
  const id = Number(taskID);
  if (!Number.isFinite(id) || id <= 0) return;
  const next = new Set(selectedDrainEventTaskIDs.value);
  if (checked) next.add(id);
  else next.delete(id);
  selectedDrainEventTaskIDs.value = next;
}

function toggleDrainSchedulerJob(jobUUID: any, checked: boolean) {
  const uuid = String(jobUUID || "").trim();
  if (!uuid) return;
  const next = new Set(selectedDrainSchedulerJobUUIDs.value);
  if (checked) next.add(uuid);
  else next.delete(uuid);
  selectedDrainSchedulerJobUUIDs.value = next;
}

function selectCurrentDrainBlockerPage(kind?: "event_task" | "scheduler_job") {
  if (!kind || kind === "event_task") {
    const next = new Set(selectedDrainEventTaskIDs.value);
    drainBlockerEventTasks.value
      .map((task: any) => Number(task.id))
      .filter((taskID: number) => Number.isFinite(taskID) && taskID > 0)
      .forEach((taskID: number) => next.add(taskID));
    selectedDrainEventTaskIDs.value = next;
  }
  if (!kind || kind === "scheduler_job") {
    const next = new Set(selectedDrainSchedulerJobUUIDs.value);
    drainBlockerSchedulerJobs.value
      .map((job: any) => String(job.uuid || "").trim())
      .filter(Boolean)
      .forEach((jobUUID: string) => next.add(jobUUID));
    selectedDrainSchedulerJobUUIDs.value = next;
  }
}

function clearSelectedDrainBlockers() {
  selectedDrainEventTaskIDs.value = new Set();
  selectedDrainSchedulerJobUUIDs.value = new Set();
}

watch(
  () => plugin.value?.icon,
  () => {
    imageError.value = false;
  }
);

function handleImageError() {
  imageError.value = true;
}

async function openDrainBlockers() {
  drainBlockersOpen.value = true;
  eventTaskPagination.page = 1;
  schedulerJobPagination.page = 1;
  await Promise.all([
    loadDrainBlockers("event_task"),
    loadDrainBlockers("scheduler_job"),
  ]);
  await refreshPluginRuntimeState();
}

async function loadDrainBlockers(kind: "event_task" | "scheduler_job") {
  if (!isRoot.value || !id.value) return;
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const pagination = kind === "event_task" ? eventTaskPagination : schedulerJobPagination;
    const resp: any = await svc.listDrainBlockers(id.value, {
      kind,
      page: pagination.page,
      page_size: pagination.pageSize,
    });
    const rows = Array.isArray(resp?.items) ? resp.items : [];
    const page = resp?.pagination || {};
    pagination.total = Number(page.total || rows.length || 0);
    pagination.page = Number(page.page || pagination.page || 1);
    pagination.pageSize = Number(page.page_size || page.pageSize || pagination.pageSize || 20);
    if (kind === "event_task") {
      drainBlockerEventTaskRows.value = rows;
    } else {
      drainBlockerSchedulerJobRows.value = rows;
    }
  } catch (err) {
    console.error("load drain blockers failed:", err);
    toast.add({
      title: "加载阻断任务失败",
      description: err?.message || String(err),
      color: "error",
      icon: "i-heroicons-exclamation-triangle",
    });
  }
}

async function refreshStatus() {
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const s: any = await svc.status(id.value);
    sysStatus.value = typeof s === "string" ? s : s?.state || s?.status || "";
    currentVersion.value = typeof s?.version === "string" ? s.version : "";
    // 优先后端字段，其次根据状态推断：仅 enabled/running 视为启用，installed/default 视为未启用
    if (s?.enabled !== undefined) {
      sysEnabled.value = Boolean(s.enabled);
    } else if (s?.isSystemEnabled !== undefined) {
      sysEnabled.value = Boolean(s.isSystemEnabled);
    } else {
      const st = (sysStatus.value || "").toLowerCase();
      sysEnabled.value = st === "enabled" || st === "running" || st === "active";
    }
  } catch (e) {
    console.warn("load status failed:", e);
  }
}

async function toggleEnable() {
  if (!canToggleSystem.value) {
    toast.add({
      title: "插件正在准备卸载",
      description: "drain 完成前不能启用、停用或改变系统运行状态。",
      color: "warning",
      icon: "i-heroicons-exclamation-triangle",
    });
    return;
  }
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();

    if (sysEnabled.value) {
      // 停用时需要确认
      const { useConfirm } = await import("~/composables/useConfirm");
      const { confirm } = useConfirm();
      const ok = await confirm({
        title: "停用插件",
        description: "停用后该插件将无法为任何租户提供服务。",
        message: "确定要停用该插件吗？",
        confirmLabel: "停用",
        cancelLabel: "取消",
        tone: "warning",
      });
      if (!ok) return;
      await svc.disable(id.value);
    } else {
      // 启用：先提示，再调用接口并轮询状态
      const pending = toast.add({
        title: "正在启用插件…",
        color: "info",
        icon: "i-heroicons-arrow-path",
        timeout: 0,
      });
      await svc.enable(id.value);
      await pollStatusUntil(true);
      toast.remove(pending.id);
      toast.add({
        title: "插件已启用",
        color: "success",
        icon: "i-heroicons-check-circle",
      });
    }

    menuRefreshToken.value += 1;
    await refreshPluginLifecycleState();
  } catch (e) {
    console.error("toggle enable failed:", e);
    toast.add({
      title: "操作失败",
      description: e?.message || String(e),
      color: "error",
    });
  }
}

async function pollStatusUntil(targetEnabled: boolean, maxAttempts = 15, delayMs = 2000) {
  for (let i = 0; i < maxAttempts; i++) {
    await refreshStatus();
    const ok = targetEnabled ? sysEnabled.value : !sysEnabled.value;
    if (ok) return;
    await new Promise((res) => setTimeout(res, delayMs));
  }
}

let unsubscribePluginDrainNotification: (() => void) | null = null;
let drainRefreshTimer: ReturnType<typeof setTimeout> | null = null;
let lastDrainNotificationKey = "";
let suppressDrainNotificationUntil = 0;

function scheduleDrainLifecycleRefresh(payload: any) {
  if (Date.now() < suppressDrainNotificationUntil) return;
  const status = String(payload?.status || "").trim();
  const jobID = String(payload?.job_id || "").trim();
  const key = `${jobID}:${status}`;
  if (key && key === lastDrainNotificationKey) return;
  lastDrainNotificationKey = key;
  if (drainRefreshTimer) clearTimeout(drainRefreshTimer);
  drainRefreshTimer = setTimeout(async () => {
    drainRefreshTimer = null;
    await refreshPluginLifecycleState({ refreshMeta: false });
  }, 500);
}

onMounted(async () => {
  await loadPluginMeta({ updateDetail: true });
  await refreshPluginLifecycleState({ refreshMeta: false });
  unsubscribePluginDrainNotification = wsBus.subscribe("_topic.system.notification", async (payload: any) => {
    if (!payload || payload.kind !== "plugin.drain.status") return;
    if (String(payload.plugin_id || "").trim() !== id.value) return;
    scheduleDrainLifecycleRefresh(payload);
  });
});

onBeforeUnmount(() => {
  if (drainRefreshTimer) {
    clearTimeout(drainRefreshTimer);
    drainRefreshTimer = null;
  }
  if (unsubscribePluginDrainNotification) {
    unsubscribePluginDrainNotification();
    unsubscribePluginDrainNotification = null;
  }
});

async function loadPluginMeta(options?: { updateDetail?: boolean }) {
  const pluginID = id.value;
  const existing = pluginDetailMetaRefreshPromises.get(pluginID);
  if (existing) return existing;
  const promise = (async () => {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const list = await svc.getMarketplace();
    const item = Array.isArray(list)
      ? (list as any[]).find(
          (p) => String(p.id || p.slug || p.name || "") === id.value
        )
      : undefined;
    if (item) {
      pluginMissing.value = false;
      if (options?.updateDetail) {
        plugin.value = {
          id: String(item.id || item.slug || item.name || ""),
          name: item.name || item.id || "-",
          description: item.description || "",
          version: item.version || "-",
          author: item.author || "",
          category: item.category || "",
          installs: Number(item.installs || item.downloadCount || 0),
          icon: item.icon,
          tags: Array.isArray(item.tags) ? item.tags : [],
        };
      }
      sysInstalled.value = !!(item as any).isSystemInstalled;
      if ((item as any).isSystemEnabled !== undefined)
        sysEnabled.value = !!(item as any).isSystemEnabled;
    } else {
      pluginMissing.value = true;
      sysInstalled.value = false;
      sysEnabled.value = false;
    }
  })().catch((e) => {
    console.warn("load plugin meta failed:", e);
  }).finally(() => {
    pluginDetailMetaRefreshPromises.delete(pluginID);
  });
  pluginDetailMetaRefreshPromises.set(pluginID, promise);
  return promise;
}

async function refreshMeta() {
  try {
    await loadPluginMeta();
  } catch (e) {
    console.warn("refresh meta failed:", e);
  }
}

async function refreshPluginLifecycleState(options?: { refreshMeta?: boolean; force?: boolean; refreshDrainProgress?: boolean }) {
  const pluginID = id.value;
  const existing = pluginDetailLifecycleRefreshPromises.get(pluginID);
  if (existing) return existing;
  const lastRefreshAt = pluginDetailLastLifecycleRefreshAt.get(pluginID) || 0;
  if (!options?.force && Date.now() - lastRefreshAt < 1000) return;
  const promise = (async () => {
    if (options?.refreshMeta !== false) {
      await refreshMeta();
    }
    if (pluginMissing.value) {
      drainJobs.value = [];
      tenantEnabled.value = false;
      tenantStatus.value = "";
      clientId.value = "";
      return;
    }
    await refreshStatus();
    await refreshDrainJobs();
    if (options?.refreshDrainProgress) {
      await refreshLatestDrainJobProgress();
      await refreshDrainJobs();
    }
    await refreshTenant();
    drainBlockerEventTaskRows.value = [];
    drainBlockerSchedulerJobRows.value = [];
    eventTaskPagination.total = 0;
    schedulerJobPagination.total = 0;
    clearSelectedDrainBlockers();
  })().finally(() => {
    pluginDetailLastLifecycleRefreshAt.set(pluginID, Date.now());
    pluginDetailLifecycleRefreshPromises.delete(pluginID);
  });
  pluginDetailLifecycleRefreshPromises.set(pluginID, promise);
  return promise;
}

async function refreshPluginRuntimeState() {
  await refreshPluginLifecycleState({ refreshMeta: false, force: true, refreshDrainProgress: true });
}

async function refreshLatestDrainJobProgress() {
  const jobID = String(latestDrainJob.value?.job_id || latestDrainJob.value?.id || "").trim();
  if (!isRoot.value || !jobID) return;
  const { useAdminPluginsService } = await import(
    "~/composables/api/services/adminPluginsService"
  );
  const svc = useAdminPluginsService();
  await svc.refreshDrainJob(jobID);
}

function suppressNearbyDrainNotification(ms = 1500) {
  suppressDrainNotificationUntil = Date.now() + ms;
  if (drainRefreshTimer) {
    clearTimeout(drainRefreshTimer);
    drainRefreshTimer = null;
  }
}

async function refreshDrainJobs() {
  if (!isRoot.value || !sysInstalled.value) {
    drainJobs.value = [];
    return;
  }
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const resp: any = await svc.listDrainJobs(id.value);
    const rows = Array.isArray(resp)
      ? resp
      : Array.isArray(resp?.items)
        ? resp.items
        : Array.isArray(resp?.data?.items)
          ? resp.data.items
          : [];
    drainJobs.value = rows;
  } catch (e) {
    console.warn("load drain jobs failed:", e);
    drainJobs.value = [];
  }
}

async function refreshTenant() {
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const conf: any = await svc.getTenantConfig(id.value);
    tenantEnabled.value = Boolean(conf?.enabled ?? conf?.isEnabled);
    tenantStatus.value = conf?.status || "";
    clientId.value = conf?.client_id || conf?.clientId || conf?.config?.client_id || clientId.value || "";
  } catch (e) {
    console.warn("load tenant config failed:", e);
  }
}

async function toggleTenant() {
  if (!canToggleTenant.value) {
    toast.add({
      title: "插件正在准备卸载",
      description: "当前插件不再允许新增租户订阅；已订阅租户只能取消订阅或删除配置。",
      color: "warning",
      icon: "i-heroicons-exclamation-triangle",
    });
    return;
  }
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    if (tenantEnabled.value) {
      const { useConfirm } = await import("~/composables/useConfirm");
      const { confirm } = useConfirm();
      const ok = await confirm({
        title: "取消订阅",
        description: "仅取消当前租户订阅，不会停止平台插件进程。",
        message: "确定要取消当前租户对该插件的订阅吗？",
        confirmLabel: "取消订阅",
        cancelLabel: "取消",
        tone: "warning",
      });
      if (!ok) return;
      await svc.setTenantEnabled(id.value, false);
      tenantEnabled.value = false;
    } else {
      const resp: any = await svc.setTenantEnabled(id.value, true);
      // 首次启用可能返回一次性明文 secret
      const secret = resp?.client_secret || resp?.secret || "";
      const cid = resp?.client_id || resp?.clientId || resp?.instance?.config?.client_id;
      if (cid) clientId.value = cid;
      if (secret) {
        oneTimeSecret.value = secret;
        showSecret.value = true;
      }
      tenantEnabled.value = true;
    }
  } catch (e) {
    console.error("toggle tenant failed:", e);
  }
}

async function rotateTenantSecret() {
  const { useConfirm } = await import("~/composables/useConfirm");
  const { confirm } = useConfirm();
  const ok = await confirm({
    title: "轮换凭证",
    description: "旧密钥将立即失效，请及时更新插件端配置。",
    message: "确定要为本租户轮换密钥吗？",
    confirmLabel: "轮换",
    cancelLabel: "取消",
    tone: "warning",
  });
  if (!ok) return;
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const resp: any = await svc.rotateCredentials(id.value);
    const secret = resp?.client_secret || resp?.secret || "";
    const cid = resp?.client_id || resp?.clientId || resp?.instance?.config?.client_id;
    if (cid) clientId.value = cid;
    if (secret) {
      const { confirm: info } = useConfirm();
      await info({
        title: "新密钥（仅此一次展示）",
        message: secret,
        confirmLabel: "已复制",
        cancelLabel: "",
        tone: "info",
      });
    }
  } catch (e) {
    console.error("rotate credentials failed:", e);
  }
}

async function deleteTenantConfig() {
  const { useConfirm } = await import("~/composables/useConfirm");
  const { confirm } = useConfirm();
  const ok = await confirm({
    title: "删除订阅配置",
    description: "删除后本租户将无法访问该插件，需重新订阅生成新凭证。",
    message: "确定删除当前租户的订阅配置吗？",
    confirmLabel: "删除",
    cancelLabel: "取消",
    tone: "danger",
  });
  if (!ok) return;
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    await svc.deleteTenantConfig(id.value);
    tenantEnabled.value = false;
    clientId.value = "";
  } catch (e) {
    console.error("delete tenant config failed:", e);
  }
}

async function restartPlugin() {
  if (!canMutateSystemRuntime.value) {
    toast.add({
      title: "插件正在准备卸载",
      description: "drain 完成前不能重启插件运行时。",
      color: "warning",
      icon: "i-heroicons-exclamation-triangle",
    });
    return;
  }
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    await svc.restart(id.value);
    await refreshPluginRuntimeState();
  } catch (e) {
    console.error("restart failed:", e);
  }
}

async function switchVersion() {
  if (!canMutateSystemRuntime.value) {
    toast.add({
      title: "插件正在准备卸载",
      description: "drain 完成前不能切换插件版本。",
      color: "warning",
      icon: "i-heroicons-exclamation-triangle",
    });
    return;
  }
  const { useOverlay } = await import("#imports");
  const overlay = useOverlay();
  const modal = overlay.create(LazyPluginsSwitchVersionModal);
  const instance = modal.open({
    pluginId: id.value,
    currentVersion: plugin.value?.version,
  });
  const version = await instance.result;
  if (!version) return;
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    await svc.switchVersion(id.value, version, { enable: true });
    await refreshPluginRuntimeState();
  } catch (e) {
    console.error("switch version failed:", e);
  }
}

function formatCount(n: number) {
  if (n >= 10000) return (n / 10000).toFixed(1) + "w";
  if (n >= 1000) return (n / 1000).toFixed(1) + "k";
  return String(n);
}

async function onInstalled(_payload?: {
  plugin: MarketplacePlugin | null;
  state: any;
}) {
  try {
    installOpen.value = false;
    await refreshPluginLifecycleState();
  } catch (e) {
    console.error("Installed refresh failed:", e);
  }
}

// 角色
const userStore = useUserStore();
const isRoot = computed(() => userStore.isRoot);
const isTenantAdmin = computed(() => userStore.isCurrentTenantAdmin);

async function uninstallPlugin() {
  if (!canFinalUninstall.value) {
    if (canStartDrain.value) {
      await startDrain();
      return;
    }
    toast.add({
      title: "还不能最终卸载",
      description: "插件仍在 drain 中，必须等所有租户实例进入 drained 后才能最终卸载。",
      color: "warning",
      icon: "i-heroicons-clock",
    });
    return;
  }
  const { useConfirm } = await import("~/composables/useConfirm");
  const { confirm } = useConfirm();
  const ok = await confirm({
    title: "最终卸载插件",
    description: "仅当租户实例全部 drained 后才允许执行。此操作会移除插件系统安装记录。",
    message: "确定最终卸载该插件？",
    confirmLabel: "最终卸载",
    cancelLabel: "取消",
    tone: "danger",
  });
  if (!ok) return;
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const version = (currentVersion.value || plugin.value?.version || "").trim();
    const purge = await confirm({
      title: "清理磁盘产物？",
      description: "最终卸载成功后可同时删除磁盘产物。",
      message: "是否在卸载后同时删除磁盘产物？",
      confirmLabel: "清理并卸载",
      cancelLabel: "仅卸载",
      tone: "warning",
    });
    const payload: Record<string, any> = { purge };
    if (version && version !== "-") {
      payload.version = version;
    }
    await svc.uninstall(id.value, payload);
    menuRefreshToken.value += 1;
    pluginMissing.value = true;
    sysInstalled.value = false;
    sysEnabled.value = false;
    drainJobs.value = [];
    await router.replace("/plugins/market");
  } catch (e) {
    console.error("uninstall failed:", e);
    if (isPluginDrainRequiredError(e)) {
      toast.add({
        title: "仍需 drain",
        description: "后端仍检测到租户实例未 drained，请等待 drain 完成后再最终卸载。",
        color: "warning",
        icon: "i-heroicons-exclamation-triangle",
      });
      await refreshPluginRuntimeState();
      return;
    }
    toast.add({
      title: "卸载失败",
      description: e?.message || String(e),
      color: "error",
      icon: "i-heroicons-exclamation-triangle",
    });
  }
}

async function handleUninstallAction() {
  if (canStartDrain.value) {
    await startDrain();
    return;
  }
  if (canFinalUninstall.value) {
    await uninstallPlugin();
    return;
  }
  toast.add({
    title: "等待 drain 完成",
    description: "插件正在阻断新使用入口并等待存量租户实例退出。",
    color: "warning",
    icon: "i-heroicons-clock",
  });
}

function isPluginDrainRequiredError(error: any) {
  const data = error?.cause?.data ?? error?.cause?.response?._data ?? error?.response?._data ?? error?.data;
  const text = [
    error?.message,
    data?.error_code,
    data?.code,
    data?.error,
    data?.message,
  ]
    .filter(Boolean)
    .join(" ");
  return text.includes("PLUGIN_DRAIN_REQUIRED");
}

async function startDrain() {
  if (!canStartDrain.value) {
    toast.add({
      title: "不能发起准备卸载",
      description: isDrainActive.value
        ? "插件已经处于准备卸载中。"
        : "插件已 drained，可直接执行最终卸载。",
      color: "warning",
      icon: "i-heroicons-exclamation-triangle",
    });
    return;
  }
  const { useConfirm } = await import("~/composables/useConfirm");
  const { confirm } = useConfirm();
  const ok = await confirm({
    title: "准备卸载插件",
    description: "系统将停止该插件的新使用入口，并等待现有租户实例完成退出。完成后才能最终卸载。",
    message: "是否现在发起准备卸载？",
    confirmLabel: "发起准备卸载",
    cancelLabel: "取消",
    tone: "warning",
  });
  if (!ok) return;
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const version = (currentVersion.value || plugin.value?.version || "").trim();
    await svc.createDrainJob(id.value, {
      version: version && version !== "-" ? version : undefined,
      reason: "root uninstall requested",
      mode: "normal",
    });
    suppressNearbyDrainNotification();
    toast.add({
      title: "已进入准备卸载",
      description: "新增订阅、调度任务和事件写入会被阻断；存量任务可继续完成。",
      color: "success",
      icon: "i-heroicons-check-circle",
    });
    await refreshPluginRuntimeState();
  } catch (err) {
    console.error("create drain job failed:", err);
    toast.add({
      title: "准备卸载失败",
      description: err?.message || String(err),
      color: "error",
      icon: "i-heroicons-exclamation-triangle",
    });
  }
}

async function cancelDrainBlockers() {
  if (selectedDrainBlockerCount.value === 0) {
    toast.add({
      title: "请选择任务",
      description: "先勾选要取消的 Event Task 或 Scheduler Job。",
      color: "warning",
      icon: "i-heroicons-exclamation-triangle",
    });
    return;
  }
  const { useConfirm } = await import("~/composables/useConfirm");
  const { confirm } = useConfirm();
  const ok = await confirm({
    title: "取消未完成任务并继续卸载",
    description: "仅用于 root 主动卸载插件时清理未完成的插件运行任务。被取消的任务不会继续执行。",
    message: `${drainBlockerRecommendation.value}\n\n将取消选中的 ${selectedDrainBlockerCount.value} 个任务。`,
    confirmLabel: "取消任务",
    cancelLabel: "取消",
    tone: "danger",
  });
  if (!ok) return;
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const result: any = await svc.cancelDrainBlockers(id.value, {
      reason: "cancelled by root drain operation",
      event_task_ids: Array.from(selectedDrainEventTaskIDs.value),
      scheduler_job_uuids: Array.from(selectedDrainSchedulerJobUUIDs.value),
    });
    suppressNearbyDrainNotification();
    toast.add({
      title: "阻断任务已取消",
      description: `event task ${result?.cancelled_event_tasks || 0}，scheduler job ${result?.cancelled_scheduler_jobs || 0}`,
      color: "success",
      icon: "i-heroicons-check-circle",
    });
    clearSelectedDrainBlockers();
    await refreshPluginRuntimeState();
    if (drainBlockersOpen.value) {
      await Promise.all([
        loadDrainBlockers("event_task"),
        loadDrainBlockers("scheduler_job"),
      ]);
    }
  } catch (err) {
    console.error("cancel drain blockers failed:", err);
    toast.add({
      title: "取消阻断任务失败",
      description: err?.message || String(err),
      color: "error",
      icon: "i-heroicons-exclamation-triangle",
    });
  }
}

async function openLogs() {
  const { useOverlay } = await import("#imports");
  const overlay = useOverlay();
  const modal = overlay.create(LazyPluginsLogsModal);
  modal.open({ pluginId: id.value });
}
</script>
