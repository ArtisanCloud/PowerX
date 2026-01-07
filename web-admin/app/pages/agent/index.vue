<script setup lang="ts">
import type { Agent } from "~/types/agent";
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

definePageMeta({
  title: "Agent 对话",
  icon: "i-heroicons-chat-bubble-left-right",
  order: 1,
});

const { t } = useI18n();
const { confirm } = useConfirm();
const { prompt } = usePrompt();
const { get, put, delete: del } = useApiClient();
const envStore = useEnvStore();
const ENV = computed(() => envStore.currentEnv || "dev");

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

// 设置消息回调
chat.onMessage = (message) => {
  // console.log("[Agent page] 收到消息:", message);
};

chat.onError = (error) => {
  console.error("聊天错误:", error);
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
  console.log("重试最后一条消息");
};

// 初始化
onMounted(async () => {
  try {
    await agentManager.fetchAgents();
    if (agents.value && agents.value.length > 0) {
      const last = chatSessions.getLastSelectedAgentId?.() ?? null;
      const fallbackId = agents.value[0].uuid;
      const pickId =
        last && agents.value.some((a) => a.uuid === last) ? last : fallbackId;
      await handleAgentSelect(pickId);
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

const agentsList = computed(() =>
  Array.isArray(agents.value) ? agents.value : []
);

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
    // console.log("加载会话消息成功，已通过缓存同步", historyMessages);
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
    // 如果删除的是当前会话，清空消息
    if (currentSessionId.value === sessionId) {
      chat.clearMessages();
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
  console.log("置顶会话功能待实现:", payload);
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
    await loadAgentAISetting(agentId);
  } catch (error) {
    console.error("选择 Agent 失败:", error);
    notifyOnce("加载会话列表失败", error instanceof Error ? error.message : "");
  }
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
  return {
    id: agent.uuid,
    name: agent.name,
    description: agent.description,
    avatar: "",
    model: cfg?.model || "gpt-3.5-turbo",
    systemPrompt: cfg?.params?.systemPrompt ?? "",
    temperature: cfg?.params?.temperature ?? 0.7,
    maxTokens: cfg?.params?.maxTokens ?? 2000,
    topP: cfg?.params?.topP ?? 1,
    frequencyPenalty: cfg?.params?.frequencyPenalty ?? 0,
    presencePenalty: cfg?.params?.presencePenalty ?? 0,
    isActive: agent.status === "active",
    capabilities: [],
  };
});

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
          class="absolute top-4 -right-3 z-10 w-6 h-6 bg-white border border-gray-200 rounded-full shadow-sm hover:shadow-md transition-shadow flex items-center justify-center text-gray-500 hover:text-gray-700"
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
            :current-session-id="currentSessionId || undefined"
            :busy="isUiBusy"
            :sessions-by-agent="sessionsByAgent"
            :sessions-loading-by-agent="sessionsLoadingByAgent"
            :has-more-by-agent="hasMoreByAgent"
            @select="handleAgentSelect"
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
        <div class="p-4 border-b border-gray-200 bg-white">
          <ConnectionIndicators :connection="chat" />
        </div>

        <ClientOnly>
          <ChatInterface
            :messages="chat.messages"
            :is-connected="!!isConnected"
            :is-streaming="!!isStreaming"
            :is-typing="!!isTyping"
            :current-agent="currentAgentForChat"
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
