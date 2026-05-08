<script setup lang="ts">
import type { Agent } from "~/types/agent";
import type { SelectOption } from "~/composables/api/types/select";

export interface ChatSession {
  id: number | string;
  title?: string;
  lastMessage?: string;
  updatedAt?: string | number | Date;
  unread?: number;
  pinned?: boolean;
}

interface Props {
  agents?: Agent[];
  currentAgentId?: string | null;
  selectorMode?: "agent" | "team";
  selectorValue?: string | null;
  selectorOptions?: SelectOption[];
  selectorPlaceholder?: string;
  selectorLabel?: string;
  showSessions?: boolean;
  currentSessionId?: number | string; // ✅ 高亮当前会话
  loading?: boolean;
  busy?: boolean;

  // 会话数据（外部单一事实来源）
  sessionsByAgent?: Record<string, ChatSession[]>;
  sessionsLoadingByAgent?: Record<string, boolean>;
  hasMoreByAgent?: Record<string, boolean>;

  // 可选：子组件内部触发加载
  fetchSessions?: (agentId: string) => Promise<void>;
}

const emit = defineEmits<{
  select: [agentId: string];
  "create-agent": [];
  "edit-agent": [agentId: string];
  "new-session": [];
  "clear-sessions": [agentId: string];
  "select-session": [payload: { agentId: string; sessionId: number | string }];
  "delete-session": [payload: { agentId: string; sessionId: number | string }];
  "load-sessions": [agentId: string];
  "load-more-sessions": [agentId: string];
  "pin-session": [
    payload: { agentId: string; sessionId: number | string; pinned: boolean },
  ];
  "rename-session": [
    payload: { agentId: string; sessionId: number | string; title: string },
  ];
}>();

const props = withDefaults(defineProps<Props>(), {
  agents: () => [],
  loading: false,
  currentAgentId: "",
  selectorMode: "agent",
  selectorValue: "",
  selectorOptions: () => [],
  selectorPlaceholder: "",
  selectorLabel: "",
  showSessions: true,
  currentSessionId: undefined,
  busy: false,
  sessionsByAgent: () => ({}),
  sessionsLoadingByAgent: () => ({}),
  hasMoreByAgent: () => ({}),
});

const { t } = useI18n();
const isBusy = computed(() => !!props.busy);
const primaryActionLabel = computed(() =>
  props.selectorMode === "team" ? "新建任务" : (t("agent.selector.newSession") || "新会话")
);

/* ---------- 顶部：Agent 选择 + 新建 ---------- */
const effectiveSelectorOptions = computed<SelectOption[]>(() => {
  if (props.selectorOptions && props.selectorOptions.length > 0) {
    return props.selectorOptions;
  }
  if (props.selectorMode === "team") {
    return [];
  }
  return (props.agents || []).map((a) => ({
    label: a.name,
    value: a.uuid,
  }));
});

const agentOptions = computed<SelectOption[]>(() =>
  (props.agents || []).map((a) => ({
    label: a.name,
    value: a.uuid,
  }))
);

const agentOptionsWithIcon = computed(() =>
  (props.agents || []).map((a) => ({
    label: a.name,
    value: a.uuid,
    icon: getAgentIcon(a),
  }))
);

function getAgentIcon(agent: Agent) {
  if (agent.meta?.icon) return agent.meta.icon;
  if (agent.source === "core") return "i-heroicons-cog-6-tooth";
  if (agent.meta?.tags?.includes("support"))
    return "i-heroicons-chat-bubble-left-right";
  if (agent.meta?.tags?.includes("enterprise"))
    return "i-heroicons-building-office";
  return "i-heroicons-cpu-chip";
}

// ✅ 选中项改为 SelectOption（对象）
const selectedAgent = computed<SelectOption>({
  get: () => {
    const id = props.selectorValue || props.currentAgentId || "";
    const found =
      effectiveSelectorOptions.value.find((o) => String(o.value) === id) ??
      ({
        label:
          props.selectorPlaceholder ||
          props.selectorLabel ||
          t("agent.selector.pickAgent") ||
          "选择 Agent",
        value: null,
      } as SelectOption);
    return found;
  },
  set: (opt) => {
    const id = String(opt?.value || "").trim();
    if (!id) return;
    emit("select", id);
    ensureSessionsLoaded(id, { force: true });
  },
});

// 当前选中 Agent 的图标
const currentIcon = computed(() => {
  const val = selectedAgent.value?.value as string | null;
  const fromSelector = effectiveSelectorOptions.value.find(
    (o: any) => String(o.value) === String(val || "")
  ) as any;
  if (fromSelector?.icon) return String(fromSelector.icon);
  const hit = agentOptionsWithIcon.value.find((o) => o.value === val);
  return hit?.icon || "i-heroicons-cpu-chip";
});

function createSession() {
  if (isBusy.value) return;
  if (!props.currentAgentId) return;
  emit("new-session");
}

function clearSessions() {
  if (isBusy.value) return;
  if (!props.currentAgentId) return;
  emit("clear-sessions", props.currentAgentId);
}

function createAgent() {
  if (isBusy.value) return;
  emit("create-agent");
}

function editAgent() {
  if (isBusy.value) return;
  if (!props.currentAgentId) return;
  emit("edit-agent", props.currentAgentId);
}

/* ---------- 搜索框（搜会话，如 ChatGPT） ---------- */
const searchQuery = ref("");

/* ---------- 列表数据：置顶 + 最近 ---------- */
function getSessions(agentId?: string): ChatSession[] {
  if (!agentId) return [];
  return props.sessionsByAgent?.[agentId] ?? [];
}
function isSessionsLoading(agentId?: string) {
  if (!agentId) return false;
  return !!props.sessionsLoadingByAgent?.[agentId];
}
function hasMore(agentId?: string) {
  if (!agentId) return false;
  return !!props.hasMoreByAgent?.[agentId];
}

const hasAnySessions = computed(() => {
  if (!props.currentAgentId) return false;
  return (getSessions(props.currentAgentId) || []).length > 0;
});

function norm(ts?: string | number | Date) {
  if (!ts) return 0;
  return new Date(ts).getTime() || 0;
}

const filteredSessions = computed(() => {
  const list = getSessions(props.currentAgentId) || [];
  const q = searchQuery.value.trim().toLowerCase();
  const hit = (s: ChatSession) => {
    if (!q) return true;
    const t = (s.title || "").toLowerCase();
    const m = (s.lastMessage || "").toLowerCase();
    return t.includes(q) || m.includes(q);
  };
  const sorted = [...list].sort(
    (a, b) => norm(b.updatedAt) - norm(a.updatedAt)
  );
  const pinned = sorted.filter((s) => !!s.pinned && hit(s));
  const recent = sorted.filter((s) => !s.pinned && hit(s));
  return { pinned, recent };
});

/* ---------- 首次/切换时加载 ---------- */
async function ensureSessionsLoaded(
  agentId: string,
  opts: { force?: boolean } = {}
) {
  if (!opts.force && getSessions(agentId)?.length) return;
  if (props.fetchSessions) {
    try {
      await props.fetchSessions(agentId);
    } catch {}
  } else {
    emit("load-sessions", agentId);
  }
}

watch(
  () => props.currentAgentId,
  (id) => {
    if (id) ensureSessionsLoaded(id, { force: true });
  },
  { immediate: true }
);

/* ---------- 交互动作 ---------- */
function onClickSession(sid: number | string) {
  if (isBusy.value) return;
  if (!props.currentAgentId) return;
  emit("select-session", { agentId: props.currentAgentId, sessionId: sid });
}
function onDeleteSession(sid: number | string) {
  if (isBusy.value) return;
  if (!props.currentAgentId) return;
  emit("delete-session", { agentId: props.currentAgentId, sessionId: sid });
}
function onTogglePin(sid: number | string, pinned: boolean) {
  if (isBusy.value) return;
  if (!props.currentAgentId) return;
  emit("pin-session", {
    agentId: props.currentAgentId,
    sessionId: sid,
    pinned,
  });
}
async function onRenameSession(sid: number | string) {
  if (isBusy.value) return;
  if (!props.currentAgentId) return;
  const title = prompt(t("agent.selector.renameSession") || "重命名会话");
  if (title && title.trim()) {
    emit("rename-session", {
      agentId: props.currentAgentId,
      sessionId: sid,
      title: title.trim(),
    });
  }
}

function fmtTime(ts?: string | number | Date) {
  if (!ts) return "";
  const d = new Date(ts);
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  const hh = String(d.getHours()).padStart(2, "0");
  const mi = String(d.getMinutes()).padStart(2, "0");
  return `${mm}-${dd} ${hh}:${mi}`;
}
</script>

<template>
  <div class="flex h-full flex-col bg-white border-r border-gray-200">
    <!-- 顶部：Agent 选择 + 新建 -->
    <div class="p-3 border-b border-gray-200 bg-white space-y-3">
      <div class="flex items-center gap-1">
        <!-- ChatGPT风格：选择器在上方 -->
        <USelectMenu
          v-model="selectedAgent"
          :items="effectiveSelectorOptions"
          option-attribute="label"
          value-attribute="value"
          searchable
          :disabled="isBusy"
          class="flex-1"
          :placeholder="
            selectorPlaceholder ||
            selectorLabel ||
            t('agent.selector.pickAgent') ||
            '选择 Agent'
          "
        >
          <template #leading>
            <UIcon :name="currentIcon" class="w-4 h-4 text-gray-500" />
          </template>
        </USelectMenu>

        <!-- 编辑当前 Agent 按钮 -->
        <UButton
          v-if="selectorMode !== 'team'"
          icon="i-heroicons-pencil-square"
          size="sm"
          variant="ghost"
          :disabled="isBusy || !currentAgentId"
          @click="editAgent"
          :title="t('agent.selector.editAgent') || '编辑 Agent'"
        />

        <!-- 新建 Agent 按钮 -->
        <UButton
          v-if="selectorMode !== 'team'"
          icon="i-heroicons-plus-circle"
          size="sm"
          variant="outline"
          :disabled="isBusy"
          @click="createAgent"
          :title="t('agent.selector.newAgent') || '新建 Agent'"
        />
      </div>

      <!-- 新建会话 + 搜索（同行对齐） -->
      <div class="flex items-center gap-2">
        <UButton
          icon="i-heroicons-plus"
          size="sm"
          variant="solid"
          :disabled="isBusy || !currentAgentId"
          @click="createSession"
          class="shrink-0 px-3"
        >
          {{ primaryActionLabel }}
        </UButton>
        <UButton
          v-if="showSessions"
          icon="i-heroicons-trash"
          size="sm"
          variant="outline"
          color="red"
          :disabled="isBusy || !currentAgentId || !hasAnySessions"
          @click="clearSessions"
          class="shrink-0"
          :title="t('agent.selector.clearSessions') || '清空该智能体全部会话'"
        />
        <UInput
          v-if="showSessions"
          v-model="searchQuery"
          icon="i-heroicons-magnifying-glass"
          size="sm"
          :disabled="isBusy"
          :placeholder="t('agent.selector.searchSessions') || '搜索会话…'"
          class="flex-1"
        />
        <div v-else class="flex-1"></div>
      </div>
    </div>

    <!-- 会话列表 -->
    <div v-if="showSessions" class="flex-1 overflow-y-auto pb-2">
      <div v-if="!currentAgentId" class="p-6 text-center text-gray-400">
        {{
          selectorMode === "team"
            ? "请选择一个团队查看会话列表"
            : t("agent.selector.pickAgentToSeeSessions") ||
              "请选择一个 Agent 查看会话列表"
        }}
      </div>

      <template v-else>
        <div v-if="isSessionsLoading(currentAgentId)" class="p-3 space-y-2">
          <USkeleton class="h-10 w-full" v-for="i in 5" :key="i" />
        </div>

        <template v-else>
          <!-- 置顶 -->
          <div v-if="filteredSessions.pinned.length" class="px-2 pt-3">
            <div
              class="px-2 py-1 text-xs font-medium text-gray-500 uppercase tracking-wide"
            >
              {{ t("agent.selector.pinned") || "置顶" }}
            </div>
            <ul class="mt-1">
              <li
                v-for="s in filteredSessions.pinned"
                :key="String(s.id)"
                class="group flex items-center gap-2 px-3 py-2 rounded-lg mx-1"
                :class="{
                  'cursor-pointer hover:bg-gray-50': !isBusy,
                  'cursor-not-allowed opacity-60': isBusy,
                  'bg-blue-50 border-l-2 border-blue-400':
                    s.id === currentSessionId,
                }"
                @click="onClickSession(s.id)"
              >
                <UIcon
                  name="i-heroicons-bookmark"
                  class="w-4 h-4 text-amber-500 flex-shrink-0"
                />
                <div class="flex-1 min-w-0">
                  <div class="text-sm text-gray-900 truncate">
                    {{
                      s.title ||
                      s.lastMessage ||
                      t("agent.selector.untitledSession") ||
                      "未命名会话"
                    }}
                  </div>
                  <div class="text-[11px] text-gray-500 truncate">
                    {{ s.lastMessage || "" }}
                  </div>
                </div>
                <div
                  class="flex items-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity"
                >
                  <span class="text-[11px] text-gray-400">{{
                    fmtTime(s.updatedAt)
                  }}</span>
                  <UBadge
                    v-if="s.unread"
                    color="primary"
                    size="xs"
                    class="min-w-[1.25rem] justify-center"
                  >
                    {{ s.unread }}
                  </UBadge>

                  <UDropdownMenu
                    :items="[
                      [
                        {
                          label: t('agent.selector.unpin') || '取消置顶',
                          icon: 'i-heroicons-bookmark-slash',
                          disabled: isBusy,
                          click: () => onTogglePin(s.id, false),
                          onSelect: () => onTogglePin(s.id, false),
                        },
                        {
                          label: t('agent.selector.rename') || '重命名',
                          icon: 'i-heroicons-pencil',
                          disabled: isBusy,
                          click: () => onRenameSession(s.id),
                          onSelect: () => onRenameSession(s.id),
                        },
                      ],
                      [
                        {
                          label: t('agent.selector.delete') || '删除',
                          icon: 'i-heroicons-trash',
                          color: 'error',
                          disabled: isBusy,
                          click: () => onDeleteSession(s.id),
                          onSelect: () => onDeleteSession(s.id),
                        },
                      ],
                    ]"
                  >
                    <UButton
                      icon="i-heroicons-ellipsis-vertical"
                      size="xs"
                      variant="ghost"
                      :disabled="isBusy"
                      @click.stop
                    />
                  </UDropdownMenu>
                </div>
              </li>
            </ul>
          </div>

          <!-- 最近 -->
          <div class="px-2 pt-3">
            <div
              class="px-2 py-1 text-xs font-medium text-gray-500 uppercase tracking-wide"
            >
              {{ t("agent.selector.recents") || "最近" }}
            </div>

            <div
              v-if="filteredSessions.recent.length === 0"
              class="p-6 text-center text-gray-400"
            >
              {{ t("agent.selector.noSessions") || "暂无会话" }}
            </div>

            <ul v-else class="mt-1">
              <li
                v-for="s in filteredSessions.recent"
                :key="String(s.id)"
                class="group flex items-center gap-2 px-3 py-2 rounded-lg mx-1"
                :class="{
                  'cursor-pointer hover:bg-gray-50': !isBusy,
                  'cursor-not-allowed opacity-60': isBusy,
                  'bg-blue-50 border-l-2 border-blue-400':
                    s.id === currentSessionId,
                }"
                @click="onClickSession(s.id)"
              >
                <UIcon
                  name="i-heroicons-chat-bubble-left-right"
                  class="w-4 h-4 text-gray-400 flex-shrink-0"
                />
                <div class="flex-1 min-w-0">
                  <div class="text-sm text-gray-900 truncate">
                    {{
                      s.title ||
                      s.lastMessage ||
                      t("agent.selector.untitledSession") ||
                      "未命名会话"
                    }}
                  </div>
                  <div class="text-[11px] text-gray-500 truncate">
                    {{ s.lastMessage || "" }}
                  </div>
                </div>
                <div
                  class="flex items-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity"
                >
                  <span class="text-[11px] text-gray-400">{{
                    fmtTime(s.updatedAt)
                  }}</span>
                  <UBadge
                    v-if="s.unread"
                    color="primary"
                    size="xs"
                    class="min-w-[1.25rem] justify-center"
                  >
                    {{ s.unread }}
                  </UBadge>

                  <UDropdownMenu
                    :items="[
                      [
                        {
                          label: t('agent.selector.pin') || '置顶',
                          icon: 'i-heroicons-bookmark',
                          disabled: isBusy,
                          click: () => onTogglePin(s.id, true),
                          onSelect: () => onTogglePin(s.id, true),
                        },
                        {
                          label: t('agent.selector.rename') || '重命名',
                          icon: 'i-heroicons-pencil',
                          disabled: isBusy,
                          click: () => onRenameSession(s.id),
                          onSelect: () => onRenameSession(s.id),
                        },
                      ],
                      [
                        {
                          label: t('agent.selector.delete') || '删除',
                          icon: 'i-heroicons-trash',
                          color: 'error',
                          disabled: isBusy,
                          click: () => onDeleteSession(s.id),
                          onSelect: () => onDeleteSession(s.id),
                        },
                      ],
                    ]"
                  >
                    <UButton
                      icon="i-heroicons-ellipsis-vertical"
                      size="xs"
                      variant="ghost"
                      :disabled="isBusy"
                      @click.stop
                    />
                  </UDropdownMenu>
                </div>
              </li>
            </ul>

            <div v-if="hasMore(currentAgentId)" class="px-2 py-2">
              <UButton
                size="xs"
                variant="outline"
                block
                :disabled="isBusy"
                @click="emit('load-more-sessions', currentAgentId!)"
              >
                {{ t("common.loadMore") || "加载更多" }}
              </UButton>
            </div>
          </div>
        </template>
      </template>
    </div>

    <div v-else class="flex-1 overflow-y-auto"></div>

    <!-- 底部（可放设置/回收站/账号信息） -->
    <div class="p-3 border-t border-gray-200 bg-gray-50 sticky bottom-0 z-10">
      <!-- 去掉 justify-between，改用 ml-auto 推右；并防止收缩换行 -->
      <div class="flex items-center text-xs text-gray-500">
        <span class="truncate">{{
          t("agent.selector.totalCount", {
            count: (sessionsByAgent?.[currentAgentId!] || []).length || 0,
          }) ||
          `共 ${(sessionsByAgent?.[currentAgentId!] || []).length || 0} 个会话`
        }}</span>

        <!-- 这里：加 ml-auto + shrink-0，保证一直贴右且不被压到下一行 -->
        <div class="flex items-center gap-2 ml-auto shrink-0">
          <UButton
            size="xs"
            variant="ghost"
            icon="i-heroicons-cog-6-tooth"
            :disabled="isBusy"
          >
            {{ t("common.settings") || "设置" }}
          </UButton>
          <UButton
            size="xs"
            variant="ghost"
            icon="i-heroicons-trash"
            :disabled="isBusy"
          >
            {{ t("common.trash") || "回收站" }}
          </UButton>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 滚动条微调，贴近 ChatGPT 侧栏效果 */
.flex-1.overflow-y-auto::-webkit-scrollbar {
  width: 6px;
}
.flex-1.overflow-y-auto::-webkit-scrollbar-thumb {
  background: #c1c1c1;
  border-radius: 3px;
}
.flex-1.overflow-y-auto::-webkit-scrollbar-thumb:hover {
  background: #a8a8a8;
}
</style>
