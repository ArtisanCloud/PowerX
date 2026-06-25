<template>
  <div class="flex h-full min-h-[calc(100vh-64px)] flex-col bg-gray-50 p-4">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-semibold text-gray-900">插件 Agent Chat</h1>
        <p class="text-sm text-gray-500">通过 PowerX Agent Session/Stream 调试插件 Skill。</p>
      </div>
      <div class="flex items-center gap-2">
        <UInput v-model="agentId" class="w-56" placeholder="Agent UUID / ID" />
        <UButton icon="i-heroicons-plus" variant="soft" :loading="creatingSession" @click="createSession">新会话</UButton>
      </div>
    </div>

    <div class="grid min-h-0 flex-1 gap-4 lg:grid-cols-[280px_minmax(0,1fr)]">
      <aside class="rounded-lg border border-gray-200 bg-white p-4">
        <div class="text-sm font-medium text-gray-900">会话上下文</div>
        <dl class="mt-4 space-y-3 text-sm">
          <div>
            <dt class="text-xs text-gray-500">Session ID</dt>
            <dd class="break-all text-gray-900">{{ sessionId || "-" }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500">链路</dt>
            <dd class="text-gray-900">/agents/sessions -> /agents/stream/sse</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500">状态</dt>
            <dd>
              <UBadge :color="chat.sseActive.value ? 'success' : 'neutral'" variant="soft">
                {{ chat.sseActive.value ? "SSE Active" : "Ready" }}
              </UBadge>
            </dd>
          </div>
        </dl>
      </aside>

      <section class="flex min-h-0 flex-col rounded-lg border border-gray-200 bg-white">
        <div class="flex-1 space-y-3 overflow-auto p-4">
          <div v-if="chat.messages.value.length === 0" class="flex h-full items-center justify-center text-sm text-gray-500">
            发送消息后将在这里显示 Agent Runtime 事件结果。
          </div>
          <div
            v-for="message in chat.messages.value"
            :key="message.id"
            class="max-w-[82%] rounded-lg px-3 py-2 text-sm"
            :class="message.role === 'user' ? 'ml-auto bg-green-50 text-green-950' : 'bg-gray-100 text-gray-900'"
          >
            <div class="whitespace-pre-wrap break-words">{{ renderContent(message.content) }}</div>
          </div>
        </div>

        <div class="border-t border-gray-200 p-3">
          <div class="flex gap-2">
            <UTextarea
              v-model="draft"
              class="min-w-0 flex-1"
              :rows="2"
              placeholder="例如：帮我重构这个 shorts：https://example.com/a.mp4"
              @keydown.ctrl.enter.prevent="send"
              @keydown.meta.enter.prevent="send"
            />
            <UButton
              icon="i-heroicons-paper-airplane"
              aria-label="发送"
              :loading="chat.isGenerating.value"
              :disabled="!canSend"
              @click="send"
            >
              发送
            </UButton>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useDualChannelConnection } from "~/composables/agent/useDualChannelConnection";
import { useApiClient } from "~/composables/api";

definePageMeta({
  title: "插件 Agent Chat",
  layout: "default",
});

const api = useApiClient();
const agentId = ref("system_default_agent");
const sessionId = ref<string | null>(null);
const draft = ref("");
const creatingSession = ref(false);
const chat = useDualChannelConnection(agentId, sessionId);

const canSend = computed(() => Boolean(draft.value.trim() && sessionId.value && !chat.isGenerating.value));

const createSession = async () => {
  creatingSession.value = true;
  try {
    const resp: any = await api.post("/agents/sessions", {
      agent_id: agentId.value,
      title: "Plugin Local Agent Chat",
      meta: {
        source: "plugin_local_chat",
      },
    });
    const data = resp?.data || resp || {};
    sessionId.value = String(data.id || data.session_id || data.sessionId || "");
  } finally {
    creatingSession.value = false;
  }
};

const send = async () => {
  const text = draft.value.trim();
  if (!text || !sessionId.value) return;
  draft.value = "";
  await chat.sendMessage(text, undefined, {
    channel: "plugin_local_chat",
    message_id: `plugin_local_${Date.now()}`,
  });
};

const renderContent = (value: unknown) => {
  if (typeof value === "string") return value;
  return JSON.stringify(value ?? "", null, 2);
};

onMounted(async () => {
  if (!sessionId.value) {
    await createSession();
  }
});
</script>
