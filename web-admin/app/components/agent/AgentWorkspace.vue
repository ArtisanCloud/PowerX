<script setup lang="ts">
import type { Agent, AgentEffectivePermissions } from "~/types/agent";
import ChatInterface from "@/components/agent/ChatInterface.vue";
import ConfigPanel from "@/components/agent/ConfigPanel.vue";
import ConnectionIndicators from "@/components/agent/ConnectionIndicators.vue";
import AgentSidebar from "@/components/agent/AgentSidebar.vue";
import { useDualChannelConnection } from "~/composables/agent/useDualChannelConnection";
import { useAgentManager } from "~/composables/agent/useAgentManager";
import { useOneShotAlert } from "~/composables/useOneShotAlert";
import { useChatSessions } from "~/composables/agent/useChatSessions";
import { useConfirm } from "~/composables/useConfirm";
import { useApiClient } from "~/composables/api";
import { usePrompt } from "~/composables/usePrompt";
import { useEnvStore } from "~/stores/envStore";
import { useUserStore } from "~/stores/user";
import {
  useAgentTeamService,
  type AgentTeamMemberRecord,
  type AgentTeamRecord,
} from "~/composables/api/services/agentTeamService";
import type { SelectOption } from "~/composables/api/types/select";

const props = withDefaults(
  defineProps<{
    forcedWorkspaceMode?: "smart" | "team";
  }>(),
  {
    forcedWorkspaceMode: undefined,
  }
);

const { t } = useI18n();
const { confirm } = useConfirm();
const { prompt } = usePrompt();
const { get, put, delete: del } = useApiClient();
const envStore = useEnvStore();
const userStore = useUserStore();
const ENV = computed(() => envStore.currentEnv || "dev");
const route = useRoute();
const localePath = useLocalePath();
const teamService = useAgentTeamService();

const workspaceBasePath = computed(() => {
  if (route.path.endsWith("/agent/sessions")) return "/agent/sessions";
  if (route.path.endsWith("/agent/team-tasks")) return "/agent/team-tasks";
  return "/agent";
});

const workspaceMode = computed<"smart" | "team">(() => {
  if (props.forcedWorkspaceMode) return props.forcedWorkspaceMode;
  if (route.path.endsWith("/agent/sessions")) return "smart";
  if (route.path.endsWith("/agent/team-tasks")) return "team";
  const raw = String(route.query.workspace || "").trim().toLowerCase();
  return raw === "team" ? "team" : "smart";
});
const workspaceTeamId = computed(() => {
  if (route.path.endsWith("/agent/team-tasks")) {
    const raw = String(route.query.team_id || "").trim();
    return raw || "";
  }
  const raw = String(route.query.team_id || "").trim();
  return raw || "";
});

const teamOptions = ref<SelectOption[]>([]);
const teamMap = ref<Record<string, AgentTeamRecord>>({});
const teamMemberAgents = ref<Array<{
  id: number;
  name: string;
  key?: string;
  avatar?: string;
  role: string;
  isTL?: boolean;
  skillHint?: string;
}>>([]);
const teamWorkspaceNotice = ref("");

const findAgentUUIDByNumericID = (id: number) => {
  const hit = (agents.value || []).find((a) => Number(a.id) === Number(id));
  return hit?.uuid || "";
};

const syncTeamRoute = async (teamId: string) => {
  if (!teamId) return;
  if (workspaceTeamId.value === teamId) return;
  const base = workspaceBasePath.value;
  if (base === "/agent") {
    await navigateTo(
      `${localePath(base)}?workspace=team&team_id=${encodeURIComponent(teamId)}`,
      { replace: true }
    );
    return;
  }
  await navigateTo(`${localePath(base)}?team_id=${encodeURIComponent(teamId)}`, {
    replace: true,
  });
};

const loadTeamsForSelector = async () => {
  if (workspaceMode.value !== "team") return;
  let merged: AgentTeamRecord[] = [];
  teamWorkspaceNotice.value = "";
  try {
    const res = await teamService.listTeams(undefined, false);
    merged = res.items || [];
  } catch (e: any) {
    teamOptions.value = [];
    teamMap.value = {};
    teamMemberAgents.value = [];
    teamWorkspaceNotice.value = "团队列表加载失败，请先检查团队配置后重试。";
    notifyOnce("加载团队失败", e?.message || "");
    return;
  }

  const map: Record<string, AgentTeamRecord> = {};
  const options: SelectOption[] = [];
  for (const team of merged) {
    const key = String(team.id);
    if (map[key]) continue;
    map[key] = team;
    options.push({
      label: `团队: ${team.team_name}`,
      value: key,
      // @ts-expect-error UI 层会读取 icon 字段
      icon: "i-heroicons-user-group",
    });
  }

  teamMap.value = map;
  teamOptions.value = options;

  if (!options.length) {
    teamMemberAgents.value = [];
    teamWorkspaceNotice.value = "当前租户暂无可用团队，请先到“团队管理”创建团队。";
    return;
  }

  const picked =
    (workspaceTeamId.value && map[workspaceTeamId.value] && workspaceTeamId.value) ||
    (options[0]?.value ? String(options[0].value) : "");
  if (!picked) return;

  if (workspaceTeamId.value && !map[workspaceTeamId.value]) {
    teamWorkspaceNotice.value = `team_id=${workspaceTeamId.value} 不存在或不可用，已自动切换到默认团队。`;
  }

  const pickedTeam = map[picked];
  if (!pickedTeam) return;
  const parentAgent = (agents.value || []).find(
    (agent) => Number(agent.id) === Number(pickedTeam.parent_agent_id)
  );
  teamMemberAgents.value = parentAgent
    ? [
        {
          id: Number(parentAgent.id),
          name: parentAgent.name,
          avatar:
            typeof parentAgent.meta?.avatar === "string"
              ? parentAgent.meta.avatar
              : undefined,
        },
      ]
    : [];
  const parentUUID = findAgentUUIDByNumericID(pickedTeam.parent_agent_id);
  if (parentUUID && currentAgentId.value !== parentUUID) {
    await handleAgentSelect(parentUUID);
  }
  if (picked !== workspaceTeamId.value) {
    await syncTeamRoute(picked);
  }
  await loadTeamMembers(picked);
};

const loadTeamMembers = async (teamId: string) => {
  const parsed = Number(teamId);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    teamMemberAgents.value = [];
    return;
  }
  try {
    const res = await teamService.listMembers(parsed);
    const members = (res.items || []) as AgentTeamMemberRecord[];
    const parentID = Number(teamMap.value[teamId]?.parent_agent_id || 0);
    const roleByAgent = new Map<number, string>();
    for (const member of members) {
      roleByAgent.set(Number(member.child_agent_id), String(member.role || "executor"));
    }
    const memberIDs = new Set<number>([
      ...members.map((m) => Number(m.child_agent_id)),
      parentID,
    ]);
    const mapped = (agents.value || [])
      .filter((agent) => memberIDs.has(Number(agent.id)))
      .sort((a, b) =>
        Number(a.id) === parentID ? -1 : Number(b.id) === parentID ? 1 : Number(a.id) - Number(b.id)
      )
      .map((agent) => ({
        id: Number(agent.id),
        name: agent.name,
        key: agent.key,
        role: Number(agent.id) === parentID ? "planner" : roleByAgent.get(Number(agent.id)) || "executor",
        isTL: Number(agent.id) === parentID,
        avatar: typeof agent.meta?.avatar === "string" ? agent.meta.avatar : undefined,
      }));
    teamMemberAgents.value = mapped;
  } catch {
    const fallbackParent = Number(teamMap.value[teamId]?.parent_agent_id || 0);
    const parent = (agents.value || []).find(
      (agent) => Number(agent.id) === fallbackParent
    );
    teamMemberAgents.value = parent
      ? [
          {
            id: Number(parent.id),
            name: parent.name,
            key: parent.key,
            role: "planner",
            isTL: true,
            avatar:
              typeof parent.meta?.avatar === "string"
                ? parent.meta.avatar
                : undefined,
          },
        ]
      : [];
  }
};
const workspaceTitle = computed(() =>
  workspaceMode.value === "team" ? "团队任务工作台" : "智能会话工作台"
);
const workspaceSubtitle = computed(() =>
  workspaceMode.value === "team"
    ? "当前为多智能体协作任务模式（主智能体 + 子智能体）。"
    : "当前为单智能体会话模式。"
);

// 会话状态管理
const chatSessions = useChatSessions();

// 状态管理
const currentAgentId = chatSessions.currentAgentId;
const currentSessionId = chatSessions.currentSessionId;
const sessionsByAgent = chatSessions.sessionsByAgent;
const sessionsLoadingByAgent = chatSessions.sessionsLoadingByAgent;
const hasMoreByAgent = chatSessions.hasMoreByAgent;

// 工具：拿到某 agent 的数组（始终给个安全数组）
// const getSessions = (agentId: number) => sessionsByAgent.value[agentId] || [];

// 双通道聊天流管理
const chat = useDualChannelConnection(currentAgentId, currentSessionId);
const runtimeLLM = ref<{ provider?: string; model?: string }>({});
const systemDefaultLLM = ref<{ provider?: string; model?: string; params?: any }>({});

// 设置消息回调
chat.onMessage = (message) => {
  const type = String(message?.type || "").toLowerCase();
  if (type !== "meta") return;
  const model = String(message?.llm_model || message?.data?.llm_model || "").trim();
  const provider = String(message?.llm_provider || message?.data?.llm_provider || "").trim();
  if (!model && !provider) return;
  runtimeLLM.value = {
    model: model || runtimeLLM.value.model,
    provider: provider || runtimeLLM.value.provider,
  };
};

chat.onError = (error) => {
  console.error("聊天错误:", error);
};

const loadSystemDefaultLLM = async () => {
  try {
    const res: any = await get("/admin/agents/settings/active", {
      params: { env: ENV.value, modality: "llm" },
      useGlobalLoading: false,
    });
    const envelope = res?.data ?? res;
    const payload = envelope?.data ?? envelope;
    const profile = payload?.profile ?? payload;
    const model = String(profile?.model || payload?.model || "").trim();
    const provider = String(profile?.provider || payload?.provider || "").trim();
    systemDefaultLLM.value = {
      model,
      provider,
      params: profile?.defaults || payload?.defaults || {},
    };
  } catch {
    systemDefaultLLM.value = {};
  }
};
const showConfigPanel = ref(false);
const editingAgent = ref<Agent | null>(null);

// 左侧面板收缩状态
const isLeftPanelCollapsed = ref(false);
const toggleLeftPanel = () => {
  isLeftPanelCollapsed.value = !isLeftPanelCollapsed.value;
};

// 一次性提醒（UAlert）
const { notifyOnce } = useOneShotAlert();

// Agent 管理
const agentManager = useAgentManager();
const { agents } = agentManager;
const effectivePermissions = ref<AgentEffectivePermissions | null>(null);
const effectivePermissionsLoading = ref(false);
const effectivePermissionsError = ref("");

const loadEffectivePermissions = async (agentId: string) => {
  const uuid = String(agentId || "").trim();
  effectivePermissions.value = null;
  effectivePermissionsError.value = "";
  if (!uuid) return;
  effectivePermissionsLoading.value = true;
  try {
    effectivePermissions.value = await agentManager.fetchMyEffectivePermissions(uuid);
  } catch (e: any) {
    effectivePermissionsError.value =
      e?.message || t("agent.effectivePermissions.loadFailed");
  } finally {
    effectivePermissionsLoading.value = false;
  }
};

const effectivePermissionItems = computed(
  () => effectivePermissions.value?.items || []
);
const effectivePermissionSummary = computed(() => {
  const items = effectivePermissionItems.value;
  const allowed = items.filter((item) => item.effective_allowed).length;
  return { allowed, total: items.length };
});
const effectivePermissionStatusColor = (allowed: boolean) =>
  allowed ? "success" : "neutral";
const effectivePermissionStatusLabel = (allowed: boolean) =>
  allowed
    ? t("agent.effectivePermissions.status.allowed")
    : t("agent.effectivePermissions.status.denied");
const effectivePermissionReasonLabel = (reason?: string) => {
  const key = String(reason || "").trim();
  if (!key) return t("agent.effectivePermissions.reason.none");
  const mapped = `agent.effectivePermissions.reason.${key}`;
  const translated = t(mapped);
  return translated === mapped ? key : translated;
};

const agentOwnerPluginID = (agent?: Agent | null) =>
  String(
    (agent as any)?.ownerPluginId ||
      (agent as any)?.owner_plugin_id ||
      agent?.meta?.pluginId ||
      ""
  ).trim();

const stripLocalSuffix = (value: string) =>
  value.endsWith(".local") ? value.slice(0, -".local".length) : value;

const hasLocalDebugCounterpart = (agent: Agent, list: Agent[]) => {
  const owner = agentOwnerPluginID(agent);
  if (!owner || owner.endsWith(".local")) return false;
  const expectedOwner = `${owner}.local`;
  const expectedKey = agent.key ? `${agent.key}.local` : "";
  return list.some((candidate) => {
    const candidateOwner = agentOwnerPluginID(candidate);
    if (candidateOwner !== expectedOwner) return false;
    if (expectedKey && candidate.key === expectedKey) return true;
    return stripLocalSuffix(candidate.key || "") === (agent.key || "");
  });
};

const preferLocalDebugAgentID = (agentId: string, list: Agent[]) => {
  const current = list.find((agent) => agent.uuid === agentId);
  if (!current) return agentId;
  const owner = agentOwnerPluginID(current);
  if (!owner || owner.endsWith(".local")) return agentId;
  const expectedOwner = `${owner}.local`;
  const expectedKey = current.key ? `${current.key}.local` : "";
  const local = list.find((candidate) => {
    const candidateOwner = agentOwnerPluginID(candidate);
    if (candidateOwner !== expectedOwner) return false;
    if (expectedKey && candidate.key === expectedKey) return true;
    if (stripLocalSuffix(candidate.key || "") === (current.key || "")) return true;
    return candidate.name === current.name;
  });
  return local?.uuid || agentId;
};

// 使用双通道聊天的状态
const isConnected = computed(() => chat.sseActive.value || chat.wsActive.value);
const isStreaming = computed(() => chat.isGenerating.value);
const isTyping = ref(false);
const isSending = ref(false);
const isUiBusy = computed(() => isSending.value || isStreaming.value);
let createSessionInFlight: Promise<any> | null = null;

// Agent 级模型覆盖（来自 /admin/agents/:uuid/ai-setting）
const agentAiSetting = ref<{ provider?: string; model?: string; params?: any } | null>(
  null
);
const loadAgentAISetting = async (agentId: string) => {
  try {
    const res: any = await get(`/admin/agents/${agentId}/ai-setting`, {
      params: { env: ENV.value },
      useGlobalLoading: false,
    });
    const payload = res?.data ?? res;
    agentAiSetting.value = {
      provider: payload?.provider,
      model: payload?.model,
      params: payload?.params,
    };
  } catch {
    agentAiSetting.value = null;
  }
};

const ensureSessionForSend = async (): Promise<string | number | null> => {
  if (currentSessionId.value) return currentSessionId.value;
  if (!currentAgentId.value) return null;
  if (!createSessionInFlight) {
    createSessionInFlight = chatSessions
      .createSession(currentAgentId.value)
      .finally(() => {
        createSessionInFlight = null;
      });
  }
  const sess: any = await createSessionInFlight;
  const id = sess?.id ?? null;
  // 兜底：确保 store 的 currentSessionId 已指向新会话（避免并发/响应形态异常导致 session_id 丢失）
  if (id != null && currentAgentId.value && currentSessionId.value !== id) {
    chatSessions.selectSession(currentAgentId.value, id);
  }
  return id;
};

const retryLastMessage = async () => {
  console.info("重试最后一条消息");
};

// 初始化
onMounted(async () => {
  try {
    await loadSystemDefaultLLM();
    await agentManager.fetchAgents();
    if (workspaceMode.value === "team") {
      await loadTeamsForSelector();
    } else if (agents.value && agents.value.length > 0) {
      const last = chatSessions.getLastSelectedAgentId?.() ?? null;
      const fallbackId = agentsList.value[0]?.uuid || agents.value[0].uuid;
      const pickId =
        last && agents.value.some((a) => a.uuid === last) ? last : fallbackId;
      await handleAgentSelect(preferLocalDebugAgentID(pickId, agents.value));
    }
  } catch (e: any) {
    if (!(e?.status === 404 || e?.statusCode === 404)) {
      notifyOnce(
        t("agent.list.loadFailed") || "加载 Agent 列表失败",
        e?.message || ""
      );
    }
  }
});

// 组件卸载时断开连接
onUnmounted(() => {
  chat.disconnect();
});

const agentsList = computed(() => {
  if (!Array.isArray(agents.value)) return [];
  return [...agents.value].sort((a, b) => {
    const aOwner = agentOwnerPluginID(a);
    const bOwner = agentOwnerPluginID(b);
    const aBaseOwner = stripLocalSuffix(aOwner);
    const bBaseOwner = stripLocalSuffix(bOwner);
    const aBaseKey = stripLocalSuffix(a.key || "");
    const bBaseKey = stripLocalSuffix(b.key || "");
    if (aBaseOwner === bBaseOwner && aBaseKey === bBaseKey) {
      const aLocal = aOwner.endsWith(".local");
      const bLocal = bOwner.endsWith(".local");
      if (aLocal !== bLocal) return aLocal ? -1 : 1;
    }
    const aHasLocal = hasLocalDebugCounterpart(a, agents.value as Agent[]);
    const bHasLocal = hasLocalDebugCounterpart(b, agents.value as Agent[]);
    if (aHasLocal !== bHasLocal) return aHasLocal ? 1 : -1;
    return 0;
  });
});

// ===== 会话事件处理 =====
const handleSelectSession = async (payload: {
  agentId: string;
  sessionId: string | number;
}) => {
  const { agentId, sessionId } = payload;
  chatSessions.selectSession(agentId, sessionId);

  // 不立即清空当前消息，等待历史加载后覆盖内存消息
  // chat.clearMessages();

  // 加载会话历史消息（会自动缓存）
  try {
    const historyMessages = await chatSessions.loadSessionMessages(sessionId);
    chat.messages.value = Array.isArray(historyMessages)
      ? [...historyMessages]
      : [];
    // console.info("加载会话消息成功，已通过缓存同步", historyMessages);
  } catch (error) {
    console.error("加载会话消息失败:", error);
    notifyOnce("加载会话消息失败", error instanceof Error ? error.message : "");
  }
};

const handleNewSession = async () => {
  if (!currentAgentId.value) return;
  try {
    const newSession = await chatSessions.createSession(currentAgentId.value);
    if (newSession) {
      chat.clearMessages();
    }
  } catch (error) {
    console.error("创建新会话失败:", error);
    notifyOnce("创建新会话失败", error instanceof Error ? error.message : "");
  }
};

const handleDeleteSession = async (payload: {
  agentId: string;
  sessionId: string | number;
}) => {
  const { agentId, sessionId } = payload;
  const wasCurrent =
    currentSessionId.value != null &&
    String(currentSessionId.value) === String(sessionId);
  const ok = await confirm({
    title: t("agent.confirmDelete") || "确定删除该会话？",
    tone: "danger",
    confirmColor: "red",
    confirmLabel: t("common.delete") || "删除",
    cancelLabel: t("common.cancel") || "取消",
  });
  if (!ok) return;

  try {
    await chatSessions.deleteSession(agentId, sessionId);
    // 如果删除的是当前会话：切到新选中的会话并刷新聊天面板；若无会话则清空面板。
    if (wasCurrent) {
      const nextSessionId = currentSessionId.value;
      if (nextSessionId != null) {
        try {
          const historyMessages = await chatSessions.loadSessionMessages(
            nextSessionId,
            true
          );
          chat.messages.value = Array.isArray(historyMessages)
            ? [...historyMessages]
            : [];
        } catch (e) {
          console.error("删除后加载新会话消息失败:", e);
          chat.clearMessages();
        }
      } else {
        chat.clearMessages();
      }
    }
  } catch (error) {
    console.error("删除会话失败:", error);
    notifyOnce("删除会话失败", error instanceof Error ? error.message : "");
  }
};

const handlePinSession = async (payload: {
  agentId: string;
  sessionId: string | number;
  pinned: boolean;
}) => {
  // TODO: 后端暂不支持置顶功能，这里先保留接口
  console.info("置顶会话功能待实现:", payload);
};

const handleLoadMoreSessions = async () => {
  if (!currentAgentId.value) return;
  try {
    await chatSessions.loadMore(currentAgentId.value);
  } catch (error) {
    console.error("加载更多会话失败:", error);
    notifyOnce("加载更多会话失败", error instanceof Error ? error.message : "");
  }
};

const handleClearAllSessions = async (agentId: string) => {
  const ok = await confirm({
    title: "清空该智能体的全部会话？",
    description: "该操作不可恢复，将删除该智能体下的所有历史会话与消息。",
    tone: "danger",
    confirmColor: "red",
    confirmLabel: t("common.delete") || "删除",
    cancelLabel: t("common.cancel") || "取消",
  });
  if (!ok) return;
  try {
    await chatSessions.clearAllSessions(agentId);
    chat.clearMessages();
  } catch (e: any) {
    notifyOnce("清空会话失败", e?.message || "");
  }
};

const handleRenameSession = async (payload: {
  agentId: string;
  sessionId: string | number;
  title: string;
}) => {
  const { agentId, sessionId, title } = payload;
  try {
    await chatSessions.renameSession(agentId, sessionId, title);
  } catch (error) {
    console.error("重命名会话失败:", error);
    notifyOnce("重命名会话失败", error instanceof Error ? error.message : "");
  }
};

const handleAgentSelect = async (agentId: string) => {
  try {
    chatSessions.selectAgent(agentId);
    await chatSessions.listSessions(agentId);
    chat.clearMessages();
    runtimeLLM.value = {};
    await loadAgentAISetting(agentId);
    await loadEffectivePermissions(agentId);
  } catch (error) {
    console.error("选择 Agent 失败:", error);
    notifyOnce("加载会话列表失败", error instanceof Error ? error.message : "");
  }
};

const handleTeamSelect = async (teamId: string) => {
  const picked = String(teamId || "").trim();
  const team = teamMap.value[picked];
  if (!picked || !team) {
    teamWorkspaceNotice.value = "请选择一个可用团队后再继续。";
    return;
  }
  teamWorkspaceNotice.value = "";
  const parentUUID = findAgentUUIDByNumericID(team.parent_agent_id);
  if (!parentUUID) {
    teamWorkspaceNotice.value = "团队主智能体不可用，请检查团队配置。";
    notifyOnce("团队主智能体不存在", `team_id=${picked} 的 parent_agent_id 未匹配到可用智能体。`);
    return;
  }
  await syncTeamRoute(picked);
  await handleAgentSelect(parentUUID);
  await loadTeamMembers(picked);
};

const handleSidebarSelect = async (value: string) => {
  if (workspaceMode.value === "team") {
    await handleTeamSelect(value);
    return;
  }
  await handleAgentSelect(value);
};

const handleSendMessage = async (content: string) => {
  if (!canSendMessage.value) return;
  if (isSending.value) return;
  if (chat.isGenerating.value) return;
  isSending.value = true;
  const meta: any = {};
  try {
    const ensuredSessionId = await ensureSessionForSend();
    // 若当前会话仍是“未命名”，用首条问题自动命名（ChatGPT 风格）
    if (currentAgentId.value && currentSessionId.value && content?.trim()) {
      const sessions = sessionsByAgent.value?.[currentAgentId.value] || [];
      const sess = sessions.find((s: any) => s.id === currentSessionId.value);
      const sessTitle = String(sess?.title || "").trim();
      const isUntitled =
        !sessTitle ||
        sessTitle === t("agent.chat.untitledSession") ||
        sessTitle.includes("未命名");
      if (isUntitled) {
        const title = String(content).trim().slice(0, 24);
        if (title) {
          await chatSessions.renameSession(
            currentAgentId.value,
            currentSessionId.value,
            title
          );
        }
      }
    }
    if (ensuredSessionId != null) meta.sessionId = ensuredSessionId;
    else if (currentSessionId.value) meta.sessionId = currentSessionId.value;
    if (currentAgentId.value) meta.agentId = currentAgentId.value;
    await chat.sendMessage(content, "chat", meta);
  } finally {
    isSending.value = false;
  }
};

const handleRetryMessage = async () => {
  await retryLastMessage();
};

const handleRegenerateFrom = async (messageId: string | number) => {
  if (isSending.value) return;
  if (chat.isGenerating.value) return;
  if (!currentSessionId.value) {
    // 没有会话就无法从历史消息重新生成
    return;
  }
  const idNum =
    typeof messageId === "number" ? messageId : parseInt(String(messageId), 10);
  if (!idNum || Number.isNaN(idNum)) return;

  isSending.value = true;
  try {
    const target = chat.messages.value.find((m: any) => m.id === idNum);
    const defaultValue =
      target && typeof target.content === "string" ? String(target.content) : "";
    const edited = await prompt({
      title: "重新编辑问题",
      description: "修改这条问题后，将从这里重新生成后续回答。",
      placeholder: "输入新的问题…",
      defaultValue,
      confirmLabel: "保存并重新生成",
      cancelLabel: "取消",
      multiline: true,
      rows: 3,
    });
    if (edited == null) return;
    await chat.regenerateFrom(idNum, "chat", edited);
  } finally {
    isSending.value = false;
  }
};

const handleClearMessages = () => {
  chat.clearMessages();
};

// 创建/编辑/保存/删除 Agent
const handleCreateAgent = () => {
  if (isUiBusy.value) {
    notifyOnce("智能体正在响应中", "请等待本次生成结束后再新建智能体。");
    return;
  }
  editingAgent.value = null;
  showConfigPanel.value = true;
};

const handleEditAgent = (agentId: string) => {
  if (isUiBusy.value) {
    notifyOnce("智能体正在响应中", "请等待本次生成结束后再编辑智能体。");
    return;
  }
  const agent = agents.value.find((a) => a.uuid === agentId);
  if (agent) {
    editingAgent.value = JSON.parse(JSON.stringify(agent)) as Agent;
    showConfigPanel.value = true;
  } else {
    console.error("[父组件] 未找到对应的 agent，agentId:", agentId);
  }
};

const handleCloseConfig = () => {
  showConfigPanel.value = false;
  editingAgent.value = null;
};

const handleDeleteAgent = async (agentId: string) => {
  const ok = await confirm({
    title: t("agent.confirmDelete") || "确定删除该会话？",
    tone: "danger",
    confirmColor: "red",
    confirmLabel: t("common.delete") || "删除",
    cancelLabel: t("common.cancel") || "取消",
  });
  if (!ok) return;
  try {
    await agentManager.deleteAgent(agentId);
    // 如果删除的是当前选中的 Agent，清空会话状态并选择第一个可用的 Agent
    if (agentId === currentAgentId.value) {
      chatSessions.clear();
      if (agents.value.length > 0) {
        const first = agents.value[0];
        if (first) await handleAgentSelect(first.uuid);
      }
    }
  } catch (error) {
    console.error("删除 Agent 失败:", error);
  }
};

const handleSaveAgent = async (config: any) => {
  if (isUiBusy.value) {
    notifyOnce("智能体正在响应中", "请等待本次生成结束后再保存配置。");
    return;
  }
  try {
    const capabilityKeys = Array.isArray(config?.capabilities)
      ? config.capabilities
          .map((c: any) => String(c?.id || c?.key || c?.name || "").trim())
          .filter(Boolean)
      : [];

    const aiParams = {
      systemPrompt: String(config?.systemPrompt || "").trim(),
      temperature:
        typeof config?.temperature === "number" ? config.temperature : undefined,
      topP: typeof config?.topP === "number" ? config.topP : undefined,
      maxTokens:
        typeof config?.maxTokens === "number" ? config.maxTokens : undefined,
      frequencyPenalty:
        typeof config?.frequencyPenalty === "number"
          ? config.frequencyPenalty
          : undefined,
      presencePenalty:
        typeof config?.presencePenalty === "number"
          ? config.presencePenalty
          : undefined,
      contextWindow:
        typeof config?.contextWindow === "number" ? config.contextWindow : undefined,
      responseFormat: config?.responseFormat || undefined,
      streaming:
        typeof config?.streaming === "boolean" ? config.streaming : undefined,
    };

    const agentUUID = String(config.uuid || config.id || "").trim();
    if (agentUUID) {
      const exist = agents.value.find((a) => a.uuid === agentUUID);
      const nextMeta = {
        ...(exist?.meta || {}),
        ...(config.meta || {}),
        ...(capabilityKeys.length ? { capabilities: capabilityKeys } : {}),
      };

      const updated = await agentManager.updateAgent(agentUUID, {
        name: config.name,
        description: config.description,
        status: config.isActive ? "active" : "disabled",
        meta: nextMeta,
      });
      const updatedUUID = String(updated?.uuid || agentUUID);

      // Agent 级 provider/model 覆盖：有选就 upsert；选“系统默认”就 delete（回退）
      if (config.useSystemModelConfig || !config.provider || !config.model) {
        try {
          await del(`/admin/agents/${updatedUUID}/ai-setting`, {
            params: { env: ENV.value },
            useGlobalLoading: false,
          });
        } catch {}
      } else {
        await put(
          `/admin/agents/${updatedUUID}/ai-setting`,
          {
            env: ENV.value,
            provider: String(config.provider),
            model: String(config.model),
            params: aiParams,
            overrideFlags: {
              provider: true,
              model: true,
              ...Object.fromEntries(
                Object.entries(aiParams).map(([k, v]) => [k, v !== undefined])
              ),
            },
            quotaPolicy: {},
          },
          { useGlobalLoading: false }
        );
      }

      await agentManager.fetchAgents();
      await loadAgentAISetting(updatedUUID);
    } else {
      const newAgent = await agentManager.createAgent({
        key: config.key || `agent_${Date.now()}`,
        name: config.name || "新建 Agent",
        description: config.description || "",
        status: config.isActive ? "active" : "disabled",
        meta: {
          ...(config.meta || {}),
          ...(capabilityKeys.length ? { capabilities: capabilityKeys } : {}),
        },
      });
      // 新建后可选写入 agent 级模型覆盖
      if (!config.useSystemModelConfig && config.provider && config.model) {
        try {
          await put(
            `/admin/agents/${newAgent.uuid}/ai-setting`,
            {
              env: ENV.value,
              provider: String(config.provider),
              model: String(config.model),
              params: aiParams,
              overrideFlags: {
                provider: true,
                model: true,
                ...Object.fromEntries(
                  Object.entries(aiParams).map(([k, v]) => [k, v !== undefined])
                ),
              },
              quotaPolicy: {},
            },
            { useGlobalLoading: false }
          );
        } catch {}
      }

      await handleAgentSelect(newAgent.uuid);
    }
    handleCloseConfig();
  } catch (error) {
    console.error("保存 Agent 失败:", error);
  }
};

// 当前 Agent
const selectedAgent = computed(() => {
  return (
    agents.value.find((agent) => agent.uuid === currentAgentId.value) || null
  );
});

// 转为 ChatInterface 需要的格式
const currentAgentForChat = computed(() => {
  const agent = selectedAgent.value;
  if (!agent) return null;
  const cfg = agentAiSetting.value;
  const runtimeModel = String(runtimeLLM.value.model || "").trim();
  const runtimeProvider = String(runtimeLLM.value.provider || "").trim();
  const configuredModel = String(cfg?.model || "").trim();
  const configuredProvider = String(cfg?.provider || "").trim();
  const defaultModel = String(systemDefaultLLM.value.model || "").trim();
  const defaultProvider = String(systemDefaultLLM.value.provider || "").trim();
  const resolvedModel = runtimeModel || configuredModel || defaultModel || "—";
  const resolvedProvider =
    runtimeProvider || configuredProvider || defaultProvider;
  const modelDisplay = resolvedProvider
    ? `${resolvedModel} (${resolvedProvider})`
    : resolvedModel;
  const defaultParams = systemDefaultLLM.value.params || {};
  const resolvedTemperature = cfg?.params?.temperature ?? defaultParams?.temperature ?? 0.7;
  const resolvedMaxTokens = cfg?.params?.maxTokens ?? defaultParams?.maxTokens ?? 2000;
  const resolvedTopP = cfg?.params?.topP ?? defaultParams?.topP ?? 1;
  return {
    id: agent.uuid,
    name: agent.name,
    description: agent.description,
    avatar: "",
    model: modelDisplay,
    systemPrompt: cfg?.params?.systemPrompt ?? "",
    temperature: resolvedTemperature,
    maxTokens: resolvedMaxTokens,
    topP: resolvedTopP,
    frequencyPenalty: cfg?.params?.frequencyPenalty ?? 0,
    presencePenalty: cfg?.params?.presencePenalty ?? 0,
    isActive: agent.status === "active",
    capabilities: [],
  };
});
const currentTenantUuid = computed(() => userStore.currentTenantUuid || "");
const hasCurrentTraceSession = computed(
  () => Boolean(currentTenantUuid.value && currentSessionId.value)
);
const currentTraceSessionUrl = computed(() => {
  if (!hasCurrentTraceSession.value) return "";
  const q = new URLSearchParams();
  q.set("tenant_uuid", currentTenantUuid.value);
  q.set("session_id", String(currentSessionId.value));
  return `${localePath("/agent/traces")}?${q.toString()}`;
});
const currentTraceSessionTitle = computed(() =>
  hasCurrentTraceSession.value
    ? "查看当前会话下所有消息的运行追踪"
    : "请先选择、新建或发送一条消息生成当前会话"
);

watch(
  () => ENV.value,
  async () => {
    await loadSystemDefaultLLM();
    if (currentAgentId.value) {
      await loadAgentAISetting(String(currentAgentId.value));
      await loadEffectivePermissions(String(currentAgentId.value));
    }
  },
  { immediate: true }
);

watch(
  () => [workspaceMode.value, workspaceTeamId.value, (agents.value || []).length],
  async () => {
    if (workspaceMode.value === "team") {
      await loadTeamsForSelector();
    } else {
      teamWorkspaceNotice.value = "";
    }
  }
);

// 允许发送消息
const canSendMessage = computed(() => {
  if (!agents.value || agents.value.length === 0) return false;
  if (!isConnected.value) return false;
  if (!selectedAgent.value) return false;
  return true;
});

// Agent 图标
const getAgentIcon = (agent: Agent) => {
  if (agent.meta?.icon) return agent.meta.icon;
  if (agent.source === "core") return "i-heroicons-cog-6-tooth";
  if (agent.meta?.tags?.includes("support"))
    return "i-heroicons-chat-bubble-left-right";
  if (agent.meta?.tags?.includes("enterprise"))
    return "i-heroicons-building-office";
  return "i-heroicons-cpu-chip";
};
</script>

<template>
  <!-- 外层：左右完全分离，中间有空隙 -->
  <div class="flex h-full gap-4 px-4 pt-4 pb-0 bg-gray-50">
    <!-- 🔌 左：插件面板（独立卡片） -->
    <AgentPluginRenderView />
    <!-- 🧱 右：主容器（独立卡片） -->
    <div
      class="flex flex-1 min-w-0 min-h-0 bg-white border border-gray-200 rounded-lg shadow-sm"
    >
      <!-- 左侧 Agent 选择器 -->
      <div
        class="relative flex-shrink-0 transition-all duration-300 ease-in-out border-r border-gray-200 min-h-0"
        :class="isLeftPanelCollapsed ? 'w-12' : 'w-80'"
      >
        <!-- 收缩/展开按钮 -->
        <button
          @click="toggleLeftPanel"
          class="absolute top-4 right-2 z-10 w-6 h-6 bg-white border border-gray-200 rounded-full shadow-sm hover:shadow-md transition-shadow flex items-center justify-center text-gray-500 hover:text-gray-700"
          :title="isLeftPanelCollapsed ? '展开面板' : '收缩面板'"
        >
          <UIcon
            :name="
              isLeftPanelCollapsed
                ? 'i-heroicons-chevron-right'
                : 'i-heroicons-chevron-left'
            "
            class="w-3 h-3"
          />
        </button>

        <!-- Agent 选择器内容 -->
        <div class="h-full min-h-0 flex flex-col">
          <AgentSidebar
            class="flex-1 min-h-0"
            v-show="!isLeftPanelCollapsed"
            :agents="agentsList"
            :current-agent-id="currentAgentId || undefined"
            :selector-mode="workspaceMode === 'team' ? 'team' : 'agent'"
            :selector-options="workspaceMode === 'team' ? teamOptions : undefined"
            :selector-value="workspaceMode === 'team' ? workspaceTeamId : undefined"
            :selector-label="workspaceMode === 'team' ? '选择团队' : '选择智能体'"
            :selector-placeholder="workspaceMode === 'team' ? '选择团队' : '选择智能体'"
            :show-sessions="true"
            :current-session-id="currentSessionId || undefined"
            :busy="isUiBusy"
            :sessions-by-agent="sessionsByAgent"
            :sessions-loading-by-agent="sessionsLoadingByAgent"
            :has-more-by-agent="hasMoreByAgent"
            @select="handleSidebarSelect"
            @create-agent="handleCreateAgent"
            @edit-agent="handleEditAgent"
            @new-session="handleNewSession"
            @clear-sessions="handleClearAllSessions"
            @select-session="handleSelectSession"
            @delete-session="handleDeleteSession"
            @pin-session="handlePinSession"
            @load-more-sessions="handleLoadMoreSessions"
            @rename-session="handleRenameSession"
          />

          <!-- 收缩状态下的简化显示 -->
          <div
            v-show="isLeftPanelCollapsed"
            class="flex-1 min-h-0 bg-white flex flex-col items-center py-4 space-y-3"
          >
            <div
              v-if="selectedAgent"
              class="w-8 h-8 rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center text-white text-xs cursor-pointer"
              :title="selectedAgent.name"
              @click="toggleLeftPanel"
            >
              <UIcon :name="getAgentIcon(selectedAgent)" class="w-4 h-4" />
            </div>
            <UButton
              icon="i-heroicons-plus"
              size="xs"
              variant="ghost"
              class="w-8 h-8 p-0 flex items-center justify-center"
              :title="'新建 Agent'"
              :disabled="isUiBusy"
              @click="handleCreateAgent"
            />
          </div>
        </div>
      </div>

      <!-- 中间聊天界面 -->
      <div class="flex-1 flex flex-col min-w-0 min-h-0">
        <div class="p-4 border-b border-gray-200 bg-white space-y-2">
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-2">
              <UBadge
                :color="workspaceMode === 'team' ? 'warning' : 'primary'"
                variant="soft"
              >
                {{ workspaceMode === "team" ? "团队任务" : "智能会话" }}
              </UBadge>
              <div>
                <div class="text-sm font-medium text-gray-900">{{ workspaceTitle }}</div>
                <div class="text-xs text-gray-500">{{ workspaceSubtitle }}</div>
              </div>
              <UPopover
                v-if="workspaceMode === 'team' && teamMemberAgents.length"
                :ui="{ content: 'w-96 p-0' }"
              >
                <button class="ml-1 flex items-center gap-2 rounded-md px-1.5 py-1 hover:bg-gray-100 dark:hover:bg-white/10" type="button">
                  <div class="flex -space-x-2">
                    <UAvatar
                      v-for="member in teamMemberAgents.slice(0, 5)"
                      :key="member.id"
                      :src="member.avatar"
                      :alt="member.name"
                      size="xs"
                      class="ring-2 ring-white"
                    />
                  </div>
                  <span class="text-xs text-gray-400">{{ teamMemberAgents.length }} 人</span>
                </button>
                <template #content>
                  <div class="rounded-lg border border-gray-200 bg-white p-3 shadow-lg dark:border-white/10 dark:bg-gray-900">
                    <div class="mb-2 flex items-center justify-between">
                      <div>
                        <div class="text-sm font-semibold text-gray-900 dark:text-gray-100">团队成员</div>
                        <div class="text-xs text-gray-500">当前协作团队</div>
                      </div>
                      <UButton size="xs" variant="ghost" icon="i-heroicons-cog-6-tooth" :to="localePath('/settings/ai/agent-teams')">
                        管理
                      </UButton>
                    </div>
                    <div class="max-h-80 space-y-2 overflow-auto">
                      <div
                        v-for="member in teamMemberAgents"
                        :key="member.id"
                        class="flex items-start gap-3 rounded-md border border-gray-100 p-2 dark:border-white/10"
                      >
                        <UAvatar :src="member.avatar" :alt="member.name" size="sm" />
                        <div class="min-w-0 flex-1">
                          <div class="flex items-center gap-2">
                            <div class="truncate text-sm font-medium text-gray-900 dark:text-gray-100">{{ member.name }}</div>
                            <UBadge size="xs" :color="member.isTL ? 'warning' : 'neutral'" variant="soft">
                              {{ member.isTL ? 'TL' : member.role }}
                            </UBadge>
                          </div>
                          <div class="truncate text-xs text-gray-500">{{ member.key || '无 Key' }}</div>
                          <div v-if="member.skillHint" class="truncate text-[11px] text-gray-400">{{ member.skillHint }}</div>
                        </div>
                      </div>
                    </div>
                  </div>
                </template>
              </UPopover>
            </div>
            <div class="flex items-center gap-2">
              <UPopover
                v-if="currentAgentId"
                :ui="{ content: 'w-[32rem] max-w-[calc(100vw-2rem)] p-0' }"
              >
                <UButton
                  size="xs"
                  variant="ghost"
                  icon="i-heroicons-shield-check"
                  :loading="effectivePermissionsLoading"
                >
                  {{ t("agent.effectivePermissions.title") }}
                  <UBadge size="xs" color="neutral" variant="soft" class="ml-1">
                    {{
                      t("agent.effectivePermissions.summary", {
                        allowed: effectivePermissionSummary.allowed,
                        total: effectivePermissionSummary.total,
                      })
                    }}
                  </UBadge>
                </UButton>
                <template #content>
                  <div class="rounded-lg border border-gray-200 bg-white shadow-lg dark:border-white/10 dark:bg-gray-900">
                    <div class="border-b border-gray-100 p-3 dark:border-white/10">
                      <div class="flex items-start justify-between gap-3">
                        <div class="min-w-0">
                          <div class="text-sm font-semibold text-gray-900 dark:text-gray-100">
                            {{ t("agent.effectivePermissions.title") }}
                          </div>
                          <div class="mt-0.5 text-xs text-gray-500">
                            {{ t("agent.effectivePermissions.description") }}
                          </div>
                        </div>
                        <UButton
                          size="xs"
                          variant="ghost"
                          icon="i-heroicons-arrow-path"
                          :loading="effectivePermissionsLoading"
                          :title="t('agent.effectivePermissions.refresh')"
                          @click="loadEffectivePermissions(String(currentAgentId || ''))"
                        />
                      </div>
                    </div>
                    <div class="max-h-96 overflow-auto p-3">
                      <UAlert
                        v-if="effectivePermissionsError"
                        color="error"
                        variant="soft"
                        icon="i-heroicons-exclamation-circle"
                        :title="t('agent.effectivePermissions.loadFailed')"
                        :description="effectivePermissionsError"
                      />
                      <div
                        v-else-if="effectivePermissionsLoading"
                        class="space-y-2"
                      >
                        <USkeleton v-for="i in 4" :key="i" class="h-14 w-full" />
                      </div>
                      <div
                        v-else-if="!effectivePermissionItems.length"
                        class="rounded-md border border-dashed border-gray-200 p-4 text-center text-sm text-gray-500 dark:border-white/10"
                      >
                        {{ t("agent.effectivePermissions.empty") }}
                      </div>
                      <div v-else class="space-y-2">
                        <div
                          v-for="item in effectivePermissionItems"
                          :key="`${item.capability_uuid}:${item.permission_code}`"
                          class="rounded-md border border-gray-100 p-3 dark:border-white/10"
                        >
                          <div class="flex items-start justify-between gap-3">
                            <div class="min-w-0">
                              <div class="truncate text-sm font-medium text-gray-900 dark:text-gray-100">
                                {{ item.display_name || item.capability_id }}
                              </div>
                              <div class="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-gray-500">
                                <UBadge size="xs" color="neutral" variant="soft">
                                  {{ item.permission_code }}
                                </UBadge>
                                <span>{{ item.plugin_id }}</span>
                              </div>
                            </div>
                            <UBadge
                              size="xs"
                              :color="effectivePermissionStatusColor(item.effective_allowed)"
                              variant="soft"
                            >
                              {{ effectivePermissionStatusLabel(item.effective_allowed) }}
                            </UBadge>
                          </div>
                          <div class="mt-2 grid grid-cols-2 gap-2 text-xs md:grid-cols-4">
                            <div>
                              <span class="text-gray-400">{{ t("agent.effectivePermissions.columns.user") }}</span>
                              <div class="font-medium" :class="item.user_allowed ? 'text-emerald-600' : 'text-gray-500'">
                                {{ effectivePermissionStatusLabel(item.user_allowed) }}
                              </div>
                            </div>
                            <div>
                              <span class="text-gray-400">{{ t("agent.effectivePermissions.columns.agent") }}</span>
                              <div class="font-medium" :class="item.agent_allowed ? 'text-emerald-600' : 'text-gray-500'">
                                {{ effectivePermissionStatusLabel(item.agent_allowed) }}
                              </div>
                            </div>
                            <div>
                              <span class="text-gray-400">{{ t("agent.effectivePermissions.columns.tenant") }}</span>
                              <div class="font-medium" :class="item.tenant_enabled ? 'text-emerald-600' : 'text-gray-500'">
                                {{ effectivePermissionStatusLabel(item.tenant_enabled) }}
                              </div>
                            </div>
                            <div>
                              <span class="text-gray-400">{{ t("agent.effectivePermissions.columns.policy") }}</span>
                              <div class="font-medium" :class="item.policy_allowed ? 'text-emerald-600' : 'text-gray-500'">
                                {{ effectivePermissionStatusLabel(item.policy_allowed) }}
                              </div>
                            </div>
                          </div>
                          <div
                            v-if="!item.effective_allowed"
                            class="mt-2 text-xs text-amber-700 dark:text-amber-300"
                          >
                            {{ t("agent.effectivePermissions.columns.reason") }}:
                            {{ effectivePermissionReasonLabel(item.deny_reason) }}
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </template>
              </UPopover>
              <UButton
                v-if="currentAgentId"
                size="xs"
                variant="ghost"
                icon="i-heroicons-bug-ant"
                :to="hasCurrentTraceSession ? currentTraceSessionUrl : undefined"
                :disabled="!hasCurrentTraceSession"
                :title="currentTraceSessionTitle"
              >
                运行追踪
              </UButton>
              <UButton
                v-if="workspaceMode !== 'smart'"
                size="xs"
                variant="ghost"
                icon="i-heroicons-chat-bubble-left-right"
                :to="localePath('/agent/sessions')"
              >
                智能会话
              </UButton>
              <UButton
                v-if="workspaceMode !== 'team'"
                size="xs"
                variant="ghost"
                icon="i-heroicons-user-group"
                :to="localePath('/agent/team-tasks')"
              >
                团队任务
              </UButton>
            </div>
          </div>
          <UAlert
            v-if="workspaceMode === 'team' && teamWorkspaceNotice"
            color="warning"
            variant="soft"
            icon="i-heroicons-exclamation-triangle"
            :title="teamWorkspaceNotice"
          >
            <template #actions>
              <UButton size="xs" variant="soft" :to="localePath('/settings/ai/agent-teams')">
                去团队管理
              </UButton>
            </template>
          </UAlert>
          <ConnectionIndicators :connection="chat" />
        </div>

        <ClientOnly>
          <ChatInterface
            :messages="chat.messages"
            :is-connected="!!isConnected"
            :is-streaming="!!isStreaming"
            :is-typing="!!isTyping"
            :current-agent="currentAgentForChat"
            :current-session-id="currentSessionId || undefined"
            :tenant-uuid="currentTenantUuid"
            :connection-indicators="true"
            :can-send-message="canSendMessage"
            @send-message="handleSendMessage"
            @retry-message="handleRetryMessage"
            @regenerate-from="handleRegenerateFrom"
            @clear-messages="handleClearMessages"
          />
        </ClientOnly>
      </div>

      <!-- 配置面板（保持挂载在主容器） -->
      <ConfigPanel
        :key="editingAgent ? editingAgent.id : 'new'"
        :agent="editingAgent"
        :is-visible="showConfigPanel"
        @close="handleCloseConfig"
        @save="handleSaveAgent"
      />
    </div>
  </div>
</template>

<style scoped>
/* 确保页面占满全高（你有顶栏时这里按需调整数值） */
.h-full {
  height: calc(100vh - 64px);
}
</style>
