<script setup lang="ts">
import type { Agent } from "~/types/agent";

// 会话类型：可按需扩展
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
  currentAgentId?: number;
  loading?: boolean;
  // ✅ 外部传入的会话数据/状态（推荐做"单一事实来源"）
  sessionsByAgent?: Record<number, ChatSession[]>;
  sessionsLoadingByAgent?: Record<number, boolean>;
  hasMoreByAgent?: Record<number, boolean>;
  // （可选）如果你希望在子组件里直接触发加载，也可传入这个函数
  fetchSessions?: (agentId: number) => Promise<void>;
}
interface Emits {
  (e: "select", agentId: number): void;
  (e: "create"): void;
  (e: "edit", agentId: number): void;
  (e: "delete", agentId: number): void;
  // ✅ 新增会话相关事件
  (
    e: "select-session",
    payload: { agentId: number; sessionId: number | string }
  ): void;
  (e: "create-session", agentId: number): void;
  (
    e: "delete-session",
    payload: { agentId: number; sessionId: number | string }
  ): void;
  (e: "load-sessions", agentId: number): void; // 若不传 fetchSessions，用这个让父组件去拉
  (e: "load-more-sessions", agentId: number): void; // 翻页
}

const props = withDefaults(defineProps<Props>(), {
  agents: () => [],
  loading: false,
  currentAgentId: 0,
  sessionsByAgent: () => ({}),
  sessionsLoadingByAgent: () => ({}),
  hasMoreByAgent: () => ({}),
});
const emit = defineEmits<Emits>();
const { t } = useI18n();

// 统一的"安全数组"
const list = computed<Agent[]>(() =>
  Array.isArray(props.agents) ? props.agents : []
);
const safeLen = computed(() => list.value.length);

// 搜索
const searchQuery = ref("");
const filteredAgents = computed<Agent[]>(() => {
  const q = searchQuery.value?.trim().toLowerCase();
  if (!q) return list.value;

  return list.value.filter((a) => {
    const name = a.name?.toLowerCase() || "";
    const desc = a.description?.toLowerCase() || "";
    const key = a.key?.toLowerCase() || "";
    const tags = (a.meta?.tags || []).join(" ").toLowerCase();
    return (
      name.includes(q) ||
      desc.includes(q) ||
      key.includes(q) ||
      tags.includes(q)
    );
  });
});

// 分组
const groupedAgents = computed(() => {
  const active = filteredAgents.value.filter((a) => a.status === "active");
  const inactive = filteredAgents.value.filter((a) => a.status === "inactive");
  return { active, inactive };
});

const selectAgent = async (id: number) => {
  emit("select", id);
  // 选中即展开
  if (!isExpanded(id)) {
    const s = new Set(expandedIds.value);
    s.add(id);
    expandedIds.value = s;
  }
  // 选中即加载（如果还没加载过）
  await ensureSessionsLoaded(id);
};

const getStatusColor = (agent: Agent) => {
  if (agent.status === "inactive") return "neutral";
  if (agent.id === props.currentAgentId) return "primary";
  return "success";
};

const getAgentIcon = (agent: Agent) => {
  // 如果有自定义图标，使用自定义图标
  if (agent.meta?.icon) {
    return agent.meta.icon;
  }

  // 根据 source 或 tags 返回默认图标
  if (agent.source === "core") return "i-heroicons-cog-6-tooth";
  if (agent.meta?.tags?.includes("support"))
    return "i-heroicons-chat-bubble-left-right";
  if (agent.meta?.tags?.includes("enterprise"))
    return "i-heroicons-building-office";

  return "i-heroicons-cpu-chip";
};

const getAgentInitials = (name: string) => {
  return (
    name
      ?.split(" ")
      .map((word) => word.charAt(0))
      .join("")
      .toUpperCase()
      .slice(0, 2) || "A"
  );
};

const canDelete = (agent: Agent) => {
  return !agent.meta?.protect_from_delete;
};

const makeMenuItems = (agent: Agent): any[][] => {
  const items: any[][] = [
    [
      {
        label: t("agent.selector.edit"),
        icon: "i-heroicons-pencil",
        onSelect: () => emit("edit", agent.id),
      },
    ],
  ];

  if (canDelete(agent)) {
    items.push([
      {
        label: t("agent.selector.delete"),
        icon: "i-heroicons-trash",
        color: "error", // 可选：高亮危险操作
        onSelect: (e?: Event) => {
          // 可选：阻止某些默认行为，比如复用快捷键时
          e?.preventDefault?.();
          emit("delete", agent.id);
        },
      },
    ]);
  }
  return items;
};

const onDropdownSelect = (item: any, agent: Agent) => {
  console.log(item);
  if (item?.value === "edit") emit("edit", agent.id);
  if (item?.value === "delete") emit("delete", agent.id);
};

// ✅ 记录哪些行处于展开态
const expandedIds = ref<Set<number>>(new Set());

const isExpanded = (id: number) => expandedIds.value.has(id);

const toggleExpand = async (id: number) => {
  // 用新 Set 触发响应式
  const s = new Set(expandedIds.value);
  s.has(id) ? s.delete(id) : s.add(id);
  expandedIds.value = s;
  // 首次展开时尝试加载该 Agent 的会话
  if (expandedIds.value.has(id)) {
    await ensureSessionsLoaded(id);
  }
};

// 工具：取会话/加载/更多标识
const getSessions = (agentId: number): ChatSession[] =>
  props.sessionsByAgent?.[agentId] ?? [];
const isSessionsLoading = (agentId: number): boolean =>
  !!props.sessionsLoadingByAgent?.[agentId];
const hasMoreSessions = (agentId: number): boolean =>
  !!props.hasMoreByAgent?.[agentId];

// 首次展开时加载
async function ensureSessionsLoaded(agentId: number) {
  if (getSessions(agentId)?.length) return; // 已有缓存
  if (props.fetchSessions) {
    try {
      await props.fetchSessions(agentId);
    } catch {}
  } else {
    emit("load-sessions", agentId);
  }
}

// 格式化时间（简单版；你也可换 dayjs）
function fmtTime(ts?: string | number | Date) {
  if (!ts) return "";
  const d = new Date(ts);
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  const hh = String(d.getHours()).padStart(2, "0");
  const mi = String(d.getMinutes()).padStart(2, "0");
  return `${mm}-${dd} ${hh}:${mi}`;
}

// 当外部切换 currentAgentId 时也自动展开+加载
watch(
  () => props.currentAgentId,
  async (id) => {
    if (!id) return;
    const s = new Set(expandedIds.value);
    s.add(id);
    expandedIds.value = s;
    await ensureSessionsLoaded(id);
  },
  { immediate: true } // 初次挂载也跑一次
);
</script>

<template>
  <div class="flex flex-col h-full bg-white border-r border-gray-200">
    <!-- 头部 -->
    <div class="p-4 border-b border-gray-200">
      <div class="flex items-center justify-between mb-3">
        <h2 class="text-lg font-semibold text-gray-900">
          {{ t("agent.selector.title") }}
        </h2>
        <UButton
          icon="i-heroicons-plus"
          size="sm"
          variant="outline"
          @click="emit('create')"
        >
          {{ t("agent.selector.create") }}
        </UButton>
      </div>

      <UInput
        v-model="searchQuery"
        :placeholder="t('agent.selector.searchPlaceholder')"
        icon="i-heroicons-magnifying-glass"
        size="sm"
        class="w-full"
      />
    </div>

    <!-- 列表 -->
    <div class="flex-1 overflow-y-auto">
      <div v-if="loading" class="p-4">
        <div class="space-y-3">
          <USkeleton class="h-16 w-full" v-for="i in 3" :key="i" />
        </div>
      </div>

      <div v-else-if="filteredAgents.length === 0" class="p-4 text-center">
        <div class="text-gray-400 mb-2">
          <UIcon
            class="w-12 h-12 mx-auto inline-block"
            name="i-heroicons-face-frown"
          />
        </div>
        <p class="text-sm text-gray-500">
          {{
            searchQuery
              ? t("agent.selector.noResults")
              : t("agent.selector.noAgents")
          }}
        </p>
      </div>

      <div v-else class="p-2">
        <!-- 活跃 -->
        <div v-if="groupedAgents.active.length > 0" class="mb-4">
          <div
            class="px-2 py-1 text-xs font-medium text-gray-500 uppercase tracking-wide"
          >
            {{ t("agent.selector.active") }}
          </div>
          <div class="space-y-1 mt-2">
            <div
              v-for="agent in groupedAgents.active"
              :key="agent.id"
              class="group relative p-3 rounded-lg cursor-pointer transition-colors duration-200"
              :class="{
                'bg-blue-50 border border-blue-200':
                  agent.id === currentAgentId,
                'hover:bg-gray-50': agent.id !== currentAgentId,
              }"
              @click="selectAgent(agent.id)"
              @dblclick.stop="emit('edit', agent.id)"
            >
              <div class="flex items-start gap-3">
                <!-- 左侧头像/图标 -->
                <div class="flex-shrink-0">
                  <div
                    class="w-10 h-10 rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center text-white"
                  >
                    <UIcon :name="getAgentIcon(agent)" class="w-5 h-5" />
                  </div>
                </div>

                <!-- 右侧主体 -->
                <div class="flex-1 min-w-0">
                  <!-- 行头部：名称 + 右侧工具区（展开按钮/状态/操作） -->
                  <div class="flex items-center gap-2">
                    <h3 class="text-sm font-medium text-gray-900 truncate">
                      {{ agent.name }}
                    </h3>

                    <!-- 展开/收起按钮（放在名称右边） -->
                    <UButton
                      icon="i-heroicons-chevron-down"
                      size="xs"
                      variant="ghost"
                      class="transition-transform ml-1"
                      :class="{ 'rotate-180': isExpanded(agent.id) }"
                      @click.stop="toggleExpand(agent.id)"
                    />

                    <!-- 右侧工具区：贴右对齐，避免覆盖 current 徽标 -->
                    <div class="flex items-center gap-1 ml-auto">
                      <UBadge
                        :color="getStatusColor(agent)"
                        size="xs"
                        class="whitespace-nowrap min-w-fit"
                      >
                        {{
                          agent.id === currentAgentId
                            ? t("agent.selector.current")
                            : t("agent.selector.available")
                        }}
                      </UBadge>

                      <!-- 直接编辑按钮（非透明，不重叠） -->
                      <UButton
                        icon="i-heroicons-pencil"
                        size="xs"
                        variant="outline"
                        class="hidden sm:inline-flex"
                        @click.stop="emit('edit', agent.id)"
                      />

                      <!-- Kebab 菜单 -->
                      <UDropdownMenu :items="makeMenuItems(agent)">
                        <UButton
                          icon="i-heroicons-ellipsis-vertical"
                          size="xs"
                          variant="outline"
                          class="hidden sm:inline-flex"
                          @click.stop
                        />
                      </UDropdownMenu>
                    </div>
                  </div>

                  <!-- 精简信息行（收起时显示） -->
                  <p
                    v-if="!isExpanded(agent.id)"
                    class="text-xs text-gray-500 mt-1 line-clamp-2"
                  >
                    {{ agent.description }}
                  </p>

                  <!-- 展开区（详尽信息 + 小屏操作按钮） -->
                  <div
                    v-show="agent.id === currentAgentId || isExpanded(agent.id)"
                    class="mt-2 space-y-2"
                  >
                    <p class="text-xs text-gray-600">{{ agent.description }}</p>

                    <div
                      class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500"
                    >
                      <span class="flex items-center">
                        <UIcon name="i-heroicons-key" class="w-3 h-3 mr-1" />
                        {{ agent.key }}
                      </span>
                      <span class="text-gray-300">•</span>
                      <span>{{ agent.source }}</span>
                      <template v-if="agent.meta?.tags?.length">
                        <span class="text-gray-300">•</span>
                        <span>{{ agent.meta.tags.join(", ") }}</span>
                      </template>
                    </div>

                    <!-- 小屏操作按钮：展开时显示，避免与徽标拥挤 -->
                    <div class="flex items-center gap-2 sm:hidden pt-1">
                      <UButton
                        size="xs"
                        icon="i-heroicons-pencil"
                        variant="outline"
                        @click.stop="emit('edit', agent.id)"
                      >
                        {{ t("agent.selector.edit") }}
                      </UButton>
                      <UButton
                        v-if="canDelete(agent)"
                        size="xs"
                        icon="i-heroicons-trash"
                        variant="outline"
                        @click.stop="emit('delete', agent.id)"
                      >
                        {{ t("agent.selector.delete") }}
                      </UButton>
                    </div>

                    <!-- ✅ 会话列表（该 Agent） -->
                    <div
                      class="mt-2 rounded-md border border-gray-200 bg-gray-50 p-2"
                    >
                      <div class="flex items-center justify-between mb-2">
                        <div class="text-xs font-medium text-gray-600">
                          {{ t("agent.selector.sessions") || "最近会话" }}
                        </div>
                        <UButton
                          size="xs"
                          variant="ghost"
                          icon="i-heroicons-plus"
                          @click.stop="emit('create-session', agent.id)"
                        >
                          {{ t("agent.selector.newSession") || "新建会话" }}
                        </UButton>
                      </div>

                      <div v-if="isSessionsLoading(agent.id)" class="space-y-2">
                        <USkeleton
                          class="h-10 w-full"
                          v-for="i in 3"
                          :key="i"
                        />
                      </div>

                      <template v-else>
                        <div
                          v-if="getSessions(agent.id).length === 0"
                          class="text-xs text-gray-400 py-2"
                        >
                          {{ t("agent.selector.noSessions") || "暂无会话" }}
                        </div>

                        <ul
                          v-else
                          class="divide-y divide-gray-200 rounded-md bg-white"
                        >
                          <li
                            v-for="s in getSessions(agent.id)"
                            :key="String(s.id)"
                            class="flex items-center gap-2 px-3 py-2 hover:bg-gray-50 cursor-pointer"
                            @click.stop="
                              emit('select-session', {
                                agentId: agent.id,
                                sessionId: s.id,
                              })
                            "
                          >
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
                            <div class="flex items-center gap-2">
                              <span class="text-[11px] text-gray-400">{{
                                fmtTime(s.updatedAt)
                              }}</span>
                              <UBadge
                                v-if="s.unread"
                                color="primary"
                                size="xs"
                                class="min-w-[1.25rem] justify-center"
                                >{{ s.unread }}</UBadge
                              >
                              <UButton
                                icon="i-heroicons-trash"
                                size="xs"
                                variant="ghost"
                                @click.stop="
                                  emit('delete-session', {
                                    agentId: agent.id,
                                    sessionId: s.id,
                                  })
                                "
                              />
                            </div>
                          </li>
                        </ul>

                        <div v-if="hasMoreSessions(agent.id)" class="pt-2">
                          <UButton
                            size="xs"
                            variant="outline"
                            block
                            @click.stop="emit('load-more-sessions', agent.id)"
                          >
                            {{ t("common.loadMore") || "加载更多" }}
                          </UButton>
                        </div>
                      </template>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 非活跃 -->
        <div v-if="groupedAgents.inactive.length > 0">
          <div
            class="px-2 py-1 text-xs font-medium text-gray-500 uppercase tracking-wide"
          >
            {{ t("agent.selector.inactive") }}
          </div>
          <div class="space-y-1 mt-2">
            <div
              v-for="agent in groupedAgents.inactive"
              :key="agent.id"
              class="group relative p-3 rounded-lg cursor-pointer transition-colors duration-200 hover:bg-gray-50 opacity-60"
              @click="selectAgent(agent.id)"
              @dblclick.stop="emit('edit', agent.id)"
            >
              <div class="flex items-start gap-3">
                <!-- 左侧头像/图标 -->
                <div class="flex-shrink-0">
                  <div
                    class="w-10 h-10 rounded-full bg-gray-400 flex items-center justify-center text-white"
                  >
                    <UIcon :name="getAgentIcon(agent)" class="w-5 h-5" />
                  </div>
                </div>

                <!-- 右侧主体 -->
                <div class="flex-1 min-w-0">
                  <!-- 行头部：名称 + 右侧工具区（展开按钮/状态/操作） -->
                  <div class="flex items-center gap-2">
                    <h3 class="text-sm font-medium text-gray-600 truncate">
                      {{ agent.name }}
                    </h3>

                    <!-- 展开/收起按钮（放在名称右边） -->
                    <UButton
                      icon="i-heroicons-chevron-down"
                      size="xs"
                      variant="ghost"
                      class="transition-transform ml-1"
                      :class="{ 'rotate-180': isExpanded(agent.id) }"
                      @click.stop="toggleExpand(agent.id)"
                    />

                    <!-- 右侧工具区：贴右对齐，避免覆盖 current 徽标 -->
                    <div class="flex items-center gap-1 ml-auto">
                      <UBadge color="neutral" size="xs">
                        {{ t("agent.selector.inactive") }}
                      </UBadge>

                      <!-- 直接编辑按钮（非透明，不重叠） -->
                      <UButton
                        icon="i-heroicons-pencil"
                        size="xs"
                        variant="outline"
                        class="hidden sm:inline-flex"
                        @click.stop="emit('edit', agent.id)"
                      />

                      <!-- Kebab 菜单 -->
                      <UDropdownMenu
                        :items="makeMenuItems(agent)"
                        @select="(item) => onDropdownSelect(item, agent)"
                      >
                        <UButton
                          icon="i-heroicons-ellipsis-vertical"
                          size="xs"
                          variant="outline"
                          class="hidden sm:inline-flex"
                          @click.stop
                        />
                      </UDropdownMenu>
                    </div>
                  </div>

                  <!-- 精简信息行（收起时显示） -->
                  <p
                    v-if="!isExpanded(agent.id)"
                    class="text-xs text-gray-400 mt-1 line-clamp-2"
                  >
                    {{ agent.description }}
                  </p>

                  <!-- 展开区（详尽信息 + 小屏操作按钮） -->
                  <div
                    v-show="agent.id === currentAgentId || isExpanded(agent.id)"
                    class="mt-2 space-y-2"
                  >
                    <p class="text-xs text-gray-500">{{ agent.description }}</p>

                    <div
                      class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-400"
                    >
                      <span class="flex items-center">
                        <UIcon name="i-heroicons-key" class="w-3 h-3 mr-1" />
                        {{ agent.key }}
                      </span>
                      <span class="text-gray-300">•</span>
                      <span>{{ agent.source }}</span>
                      <template v-if="agent.meta?.tags?.length">
                        <span class="text-gray-300">•</span>
                        <span>{{ agent.meta.tags.join(", ") }}</span>
                      </template>
                    </div>

                    <!-- 小屏操作按钮：展开时显示，避免与徽标拥挤 -->
                    <div class="flex items-center gap-2 sm:hidden pt-1">
                      <UButton
                        size="xs"
                        icon="i-heroicons-pencil"
                        variant="outline"
                        @click.stop="emit('edit', agent.id)"
                      >
                        {{ t("agent.selector.edit") }}
                      </UButton>
                      <UButton
                        v-if="canDelete(agent)"
                        size="xs"
                        icon="i-heroicons-trash"
                        variant="outline"
                        @click.stop="emit('delete', agent.id)"
                      >
                        {{ t("agent.selector.delete") }}
                      </UButton>
                    </div>

                    <!-- ✅ 会话列表（该 Agent） -->
                    <div
                      class="mt-2 rounded-md border border-gray-200 bg-gray-50 p-2"
                    >
                      <div class="flex items-center justify-between mb-2">
                        <div class="text-xs font-medium text-gray-600">
                          {{ t("agent.selector.sessions") || "最近会话" }}
                        </div>
                        <UButton
                          size="xs"
                          variant="ghost"
                          icon="i-heroicons-plus"
                          @click.stop="emit('create-session', agent.id)"
                        >
                          {{ t("agent.selector.newSession") || "新建会话" }}
                        </UButton>
                      </div>

                      <div v-if="isSessionsLoading(agent.id)" class="space-y-2">
                        <USkeleton
                          class="h-10 w-full"
                          v-for="i in 3"
                          :key="i"
                        />
                      </div>

                      <template v-else>
                        <div
                          v-if="getSessions(agent.id).length === 0"
                          class="text-xs text-gray-400 py-2"
                        >
                          {{ t("agent.selector.noSessions") || "暂无会话" }}
                        </div>

                        <ul
                          v-else
                          class="divide-y divide-gray-200 rounded-md bg-white"
                        >
                          <li
                            v-for="s in getSessions(agent.id)"
                            :key="String(s.id)"
                            class="flex items-center gap-2 px-3 py-2 hover:bg-gray-50 cursor-pointer"
                            @click.stop="
                              emit('select-session', {
                                agentId: agent.id,
                                sessionId: s.id,
                              })
                            "
                          >
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
                            <div class="flex items-center gap-2">
                              <span class="text-[11px] text-gray-400">{{
                                fmtTime(s.updatedAt)
                              }}</span>
                              <UBadge
                                v-if="s.unread"
                                color="primary"
                                size="xs"
                                class="min-w-[1.25rem] justify-center"
                                >{{ s.unread }}</UBadge
                              >
                              <UButton
                                icon="i-heroicons-trash"
                                size="xs"
                                variant="ghost"
                                @click.stop="
                                  emit('delete-session', {
                                    agentId: agent.id,
                                    sessionId: s.id,
                                  })
                                "
                              />
                            </div>
                          </li>
                        </ul>

                        <div v-if="hasMoreSessions(agent.id)" class="pt-2">
                          <UButton
                            size="xs"
                            variant="outline"
                            block
                            @click.stop="emit('load-more-sessions', agent.id)"
                          >
                            {{ t("common.loadMore") || "加载更多" }}
                          </UButton>
                        </div>
                      </template>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 底部 -->
    <div class="p-4 border-t border-gray-200 bg-gray-50">
      <div class="text-xs text-gray-500 text-center">
        {{ t("agent.selector.totalCount", { count: safeLen }) }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
