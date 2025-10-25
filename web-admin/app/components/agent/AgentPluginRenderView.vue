<script lang="ts" setup>
import { useAgentManager } from "~/composables/agent/useAgentManager";
import { useOneShotAlert } from "~/composables/useOneShotAlert";
import { useChatSessions } from "~/composables/agent/useChatSessions";
import type { Agent } from "~/types/agent";

const agentManager = useAgentManager();
const { agents } = agentManager;

const chatSessions = useChatSessions();

const currentAgentId = chatSessions.currentAgentId;

const selectedAgent = computed(() => {
  return (
    agents.value.find((agent) => agent.id === currentAgentId.value) || null
  );
});

/* ========= 插件专属侧栏：显示条件 & 收缩状态 ========= */
// 这里先强制为 true 以便开发预览；接入真实判断后改回去

const isPluginAgent = computed(() => {
  const a = selectedAgent.value as Agent | null;
  if (!a) return false;
  return (
    a.source === "plugin" ||
    a.meta?.isPlugin === true ||
    !!a.meta?.pluginId ||
    a.meta?.tags?.includes?.("plugin")
  );
});

const isPluginPanelCollapsed = ref(false);

const togglePluginPanel = () => {
  isPluginPanelCollapsed.value = !isPluginPanelCollapsed.value;

  // 会话状态管理
  const chatSessions = useChatSessions();

  watch(
    () => selectedAgent.value?.id,
    () => {
      isPluginPanelCollapsed.value = false;
    }
  );

  const currentAgentId = chatSessions.currentAgentId;

  const selectedAgent = computed(() => {
    return (
      agents.value.find((agent) => agent.id === currentAgentId.value) || null
    );
  });
};
</script>
<template>
  <div
    v-if="isPluginAgent"
    class="relative flex-shrink-0 transition-all duration-300 ease-in-out bg-white border border-gray-200 rounded-lg shadow-sm min-h-0"
    :class="isPluginPanelCollapsed ? 'w-12' : 'w-[36rem]'"
  >
    <!-- 收缩/展开按钮（浮在卡片右侧边缘） -->
    <button
      @click="togglePluginPanel"
      class="absolute top-4 -right-3 z-10 w-6 h-6 bg-white border border-gray-200 rounded-full shadow-sm hover:shadow-md transition-shadow flex items-center justify-center text-gray-500 hover:text-gray-700"
      :title="isPluginPanelCollapsed ? '展开插件面板' : '收缩插件面板'"
    >
      <UIcon
        :name="
          isPluginPanelCollapsed
            ? 'i-heroicons-chevron-right'
            : 'i-heroicons-chevron-left'
        "
        class="w-3 h-3"
      />
    </button>

    <!-- 插件内容区 -->
    <div v-show="!isPluginPanelCollapsed" class="h-full overflow-auto">
      <div class="p-4 space-y-3">
        <div class="text-xs text-gray-400 uppercase tracking-wide">
          插件面板
        </div>
        <div class="text-sm text-gray-700">
          当前 Agent：<span class="font-medium">{{ selectedAgent?.name }}</span>
        </div>
        <div class="text-xs text-gray-500">
          可在此渲染插件自定义内容（表单/看板/指标/工具面板等）。
        </div>
        <!-- TODO: 真正的插件组件 -->
        <!-- <PluginAgentPanel :agent="selectedAgent!" :session-id="currentSessionId || undefined" /> -->
      </div>
    </div>
  </div>
</template>
