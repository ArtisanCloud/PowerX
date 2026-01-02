<script setup lang="ts">
import {
  ref,
  computed,
  onMounted,
  onBeforeUnmount,
  onUpdated,
  watch,
  nextTick,
  isRef,
  type Ref,
  type DeepReadonly,
} from "vue";
import type { ChatMessage } from "~/types/message";
import type { AgentConfig } from "~/composables/agent/useAgentManager";
import MessageItem from "~/components/agent/MessageItem.vue";

type ViewAgent = AgentConfig | DeepReadonly<AgentConfig>;
type MessageArray = ChatMessage[] | ReadonlyArray<ChatMessage>;

// 允许 3 种传法：数组值 / Ref<数组> / DeepReadonly<数组>
type MessagesProp =
  | MessageArray
  | Ref<MessageArray>
  | DeepReadonly<MessageArray>;

const DEBUG = false; // 想关就设为 false

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    messages?: MessagesProp;
    isConnected?: boolean;
    isStreaming?: boolean;
    isTyping?: boolean;
    currentAgent?: ViewAgent | null;

    canSendMessage?: boolean;
    connectionIndicators?: boolean;
  }>(),
  {
    messages: () => [],
    isConnected: false,
    isStreaming: false,
    isTyping: false,
    currentAgent: null,
    canSendMessage: true,
  }
);

const emit = defineEmits<{
  (e: "send-message", content: string): void;
  (e: "retry-message"): void;
  (e: "regenerate-from", messageId: string | number): void;
  (e: "clear-messages"): void;
}>();

/* ----------------- 把 props.messages 统一归一化为只读数组 ----------------- */
const messages = computed<ReadonlyArray<ChatMessage>>(() => {
  const m = props.messages as any;
  return isRef(m) ? (m.value ?? []) : (m ?? []);
});

/* ----------------- 基础状态 ----------------- */
const { t } = useI18n();
const messageInput = ref("");
const inputRef = ref<HTMLTextAreaElement>();
const isComposing = ref(false);

const messagesContainer = ref<HTMLElement>();
const bottomSentinel = ref<HTMLDivElement>();

/* ----------------- 滚动 & 可见性 ----------------- */
const isAtBottom = ref(true);
const unreadCount = ref(0);
const DISTANCE_TOLERANCE = 10;

function calcIsAtBottom(): boolean {
  const el = messagesContainer.value;
  if (!el) return true;
  const { scrollTop, scrollHeight, clientHeight } = el;
  return scrollTop + clientHeight >= scrollHeight - DISTANCE_TOLERANCE;
}

function scrollToBottom(opts: { force?: boolean; smooth?: boolean } = {}) {
  if (!opts.force && !isAtBottom.value) return;
  bottomSentinel.value?.scrollIntoView({
    block: "end",
    behavior: opts.smooth ? "smooth" : "auto",
  });
}
function jumpToBottom() {
  scrollToBottom({ force: true, smooth: true });
  unreadCount.value = 0;
}

defineExpose({ scrollToBottom });

function handleScroll() {
  isAtBottom.value = calcIsAtBottom();
  if (isAtBottom.value) unreadCount.value = 0;
}

/* IO 判定底部 */
let io: IntersectionObserver | null = null;
function setupIO() {
  const root = messagesContainer.value;
  const target = bottomSentinel.value;
  if (!root || !target || !("IntersectionObserver" in window)) return;

  io = new IntersectionObserver(
    (entries) => {
      const e = entries[0];
      isAtBottom.value = !!e?.isIntersecting;
      if (isAtBottom.value) unreadCount.value = 0;
    },
    { root, threshold: 0 }
  );
  io.observe(target);
}

/* 媒体加载补偿 */
function bindMediaLoadScroll() {
  const root = messagesContainer.value;
  if (!root) return;
  const once = (el: HTMLElement, evt: string, cb: () => void) => {
    const handler = () => {
      el.removeEventListener(evt, handler as any);
      cb();
    };
    el.addEventListener(evt, handler as any, { once: true });
  };

  const imgs = Array.from(root.querySelectorAll("img"));
  imgs.forEach((img) => {
    const im = img as HTMLImageElement;
    if (im.complete) return;
    once(im, "load", () => scrollToBottom());
  });

  const videos = Array.from(root.querySelectorAll("video"));
  videos.forEach((video) => {
    once(video as HTMLElement, "loadedmetadata", () => scrollToBottom());
    once(video as HTMLElement, "canplay", () => scrollToBottom());
  });
}

/* 生命周期 & 监听 */
onMounted(async () => {
  await nextTick();
  setupIO();
  scrollToBottom({ force: true });
  bindMediaLoadScroll();
});

// 观察消息长度变化（使用归一化后的 messages）
watch(
  () => messages.value.length,
  (len, oldLen) => {
    if (!DEBUG) return;
    console.log("[ChatInterface] messages.length:", oldLen, "→", len);
  },
  { immediate: true }
);

// 看“最后一条”是否变化（内容长度 & isStreaming）
const lastMessage = computed(() => messages.value.at(-1));

watch(
  () => ({
    id: lastMessage.value?.id,
    role: lastMessage.value?.role,
    contentLen:
      typeof lastMessage.value?.content === "string"
        ? (lastMessage.value?.content as string).length
        : -1,
    isStreaming: (lastMessage.value as any)?.isStreaming,
    isThinking: (lastMessage.value as any)?.isThinking,
    done: (lastMessage.value as any)?.done,
  }),
  (val, oldVal) => {
    if (!DEBUG) return;
    console.log("[ChatInterface] lastMessage snapshot:", oldVal, "→", val);
  },
  { deep: false }
);

// 观察指针是否变化（仅调试：父组件是否替换了数组引用）
let _prevPtr: any = null;
watch(
  () => props.messages,
  (now) => {
    if (!DEBUG) return;
    console.log(
      "[ChatInterface] props.messages pointer changed?",
      now === _prevPtr ? "NO" : "YES",
      now
    );
    _prevPtr = now;
  },
  { deep: false, immediate: true }
);

// DOM 更新后再打一次快照（确保视图联动）
onUpdated(() => {
  if (!DEBUG) return;
  nextTick(() => {
    const cur = messages.value.at(-1);
    // console.log("[ChatInterface] onUpdated: last message now =", {
    //   id: cur?.id,
    //   role: cur?.role,
    //   content:
    //     typeof cur?.content === "string"
    //       ? (cur?.content as string).slice(-50)
    //       : cur?.content,
    //   isStreaming: (cur as any)?.isStreaming,
    //   isThinking: (cur as any)?.isThinking,
    //   done: (cur as any)?.done,
    // });
  });
});

onBeforeUnmount(() => {
  io?.disconnect();
  io = null;
  ro?.disconnect();
  ro = null;
});

// —— 关键修复：以下监听全部改用 messages（而不是 props.messages）——
watch(
  () => messages.value.length,
  async (newLen, oldLen) => {
    await nextTick();
    if (isAtBottom.value) {
      scrollToBottom();
    } else if ((newLen ?? 0) > (oldLen ?? 0)) {
      unreadCount.value++;
    }
    bindMediaLoadScroll();
  }
);

watch(
  () => messages.value.at(-1)?.content,
  async () => {
    await nextTick();
    if (isAtBottom.value) scrollToBottom();
  },
  { deep: true }
);

watch(
  () => [props.isTyping, props.isStreaming],
  async () => {
    await nextTick();
    if (isAtBottom.value) scrollToBottom();
  }
);

/* 容器 Resize 补偿滚动 */
let ro: ResizeObserver | null = null;
onMounted(() => {
  if (!DEBUG) return;
  console.log("[ChatInterface] mounted; initial messages =", messages.value);
  if ("ResizeObserver" in window) {
    ro = new ResizeObserver(() => {
      if (isAtBottom.value) scrollToBottom();
    });
    if (messagesContainer.value) ro.observe(messagesContainer.value);
  }
});

/* ----------------- 输入 & 发送 ----------------- */
function sendMessage() {
  const content = messageInput.value.trim();
  if (
    !content ||
    props.isStreaming ||
    !props.isConnected ||
    !props.canSendMessage
  )
    return;
  emit("send-message", content);
  messageInput.value = "";
  if (inputRef.value) {
    inputRef.value.style.height = "auto";
    inputRef.value.focus();
  }
}

function handleKeydown(e: KeyboardEvent) {
  // 只支持组合键发送：Shift+Enter 换行，Ctrl/Cmd+Enter 发送
  const sendByModEnter = (e.ctrlKey || e.metaKey) && e.key === "Enter";

  if (sendByModEnter) {
    e.preventDefault();
    sendMessage();
  }
  // 单独按 Enter 键时不发送消息，允许换行
}

function adjustTextareaHeight() {
  if (!inputRef.value) return;
  inputRef.value.style.height = "auto";
  inputRef.value.style.height =
    Math.min(inputRef.value.scrollHeight, 160) + "px";
}

/* 粘贴图片 -> 走图片上传（可选） */
function handlePaste(e: ClipboardEvent) {
  const items = e.clipboardData?.items;
  if (!items) return;
  for (const it of items) {
    if (it.kind === "file") {
      const file = it.getAsFile();
      if (file && file.type.startsWith("image/")) {
        console.log("粘贴图片:", file.name);
        e.preventDefault();
        break;
      }
    }
  }
}

/* ----------------- UI 辅助 ----------------- */

function getConnectionStatusText() {
  if (!props.isConnected) return t("agent.chat.disconnected");
  if (props.isStreaming) return t("agent.chat.responding");
  if (props.isTyping) return t("agent.chat.typing");
  return t("agent.chat.connected");
}
function getConnectionStatusColor() {
  if (!props.isConnected) return "text-red-500";
  if (props.isStreaming || props.isTyping) return "text-blue-500";
  return "text-green-500";
}

const showScrollBtn = computed(() => !isAtBottom.value);

/* ----------------- 录音和多功能按钮 ----------------- */
const isRecording = ref(false);

function toggleRecording() {
  isRecording.value = !isRecording.value;
  if (isRecording.value) startRecording();
  else stopRecording();
}

function startRecording() {
  console.log("开始录音");
}
function stopRecording() {
  console.log("停止录音");
}
function stopGeneration() {
  console.log("停止生成");
}

function handleFileUpload() {
  const input = document.createElement("input");
  input.type = "file";
  input.accept = ".pdf,.doc,.docx,.txt,.md";
  input.onchange = (e) => {
    const file = (e.target as HTMLInputElement).files?.[0];
    if (file) {
      console.log("上传文件:", file.name);
    }
  };
  input.click();
}

function handleImageUpload() {
  const input = document.createElement("input");
  input.type = "file";
  input.accept = "image/*";
  input.onchange = (e) => {
    const file = (e.target as HTMLInputElement).files?.[0];
    if (file) {
      console.log("上传图片:", file.name);
    }
  };
  input.click();
}

function handleCreateDocument() {
  console.log("创建文档");
}
function handleCreateChart() {
  console.log("生成图表");
}

/* ---------- ChatGPT 风格：左侧 + 面板 & 模式切换 ---------- */
type ChatMode = "default" | "code" | "vision";
const showPlusPanel = ref(false);
const chatMode = ref<ChatMode>("default");

const modeOptions: Array<{
  label: string;
  value: ChatMode;
  icon: string;
  desc?: string;
}> = [
  {
    label: "默认",
    value: "default",
    icon: "i-heroicons-sparkles",
    desc: "通用对话",
  },
  {
    label: "代码",
    value: "code",
    icon: "i-heroicons-code-bracket",
    desc: "更偏代码与技术",
  },
  {
    label: "多模态",
    value: "vision",
    icon: "i-heroicons-photo",
    desc: "图片/图表理解",
  },
];

function selectMode(m: ChatMode) {
  chatMode.value = m;
  showPlusPanel.value = false;
}
function onUploadFile() {
  handleFileUpload();
  showPlusPanel.value = false;
}
function onUploadImage() {
  handleImageUpload();
  showPlusPanel.value = false;
}
function onPlusOpenStateChange(isOpen: boolean) {
  showPlusPanel.value = isOpen;
}
function onSendClick() {
  if (props.isStreaming) stopGeneration();
  else sendMessage();
}
</script>

<template>
  <div class="flex flex-col h-full min-h-0 bg-white">
    <!-- 头部 -->
    <div class="flex-shrink-0 p-4 border-b border-gray-200 bg-white">
      <div class="flex items-center justify-between">
        <div class="flex items-center space-x-3">
          <div
            v-if="currentAgent?.avatar"
            class="w-10 h-10 rounded-full bg-cover bg-center"
            :style="{ backgroundImage: `url(${currentAgent.avatar})` }"
          />
          <div
            v-else
            class="w-10 h-10 rounded-full bg-gradient-to-br from-purple-500 to-pink-500 flex items-center justify-center text-white font-medium"
          >
            {{ currentAgent?.name?.charAt(0)?.toUpperCase() || "A" }}
          </div>

          <div>
            <h3 class="text-lg font-semibold text-gray-900">
              {{ currentAgent?.name || t("agent.chat.defaultAgent") }}
            </h3>
            <div class="flex items-center space-x-2 text-sm">
              <span :class="getConnectionStatusColor()">
                {{ getConnectionStatusText() }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 消息列表（滚动容器） -->
    <div
      ref="messagesContainer"
      class="relative flex-1 min-h-0 overflow-y-auto"
      @scroll="handleScroll"
    >
      <!-- 空状态 -->
      <div
        v-if="messages.length === 0"
        class="flex items-center justify-center h-full"
      >
        <div class="text-center">
          <div
            class="w-16 h-16 mx-auto mb-4 bg-gray-100 rounded-full flex items-center justify-center"
          >
            <UIcon
              name="i-heroicons-chat-bubble-left-right"
              class="w-8 h-8 text-gray-400"
            />
          </div>
          <h3 class="text-lg font-medium text-gray-900 mb-2">
            {{ t("agent.chat.welcomeTitle") }}
          </h3>
          <p class="text-gray-500 max-w-sm">
            {{ t("agent.chat.welcomeMessage") }}
          </p>
        </div>
      </div>

      <!-- 消息项 -->
      <div v-else class="max-w-4xl mx-auto px-4 py-6">
        <div class="space-y-6">
          <MessageItem
            v-for="message in messages"
            :key="message.id"
            :message="message"
            :is-streaming="
              (isStreaming && message === messages[messages.length - 1]) ||
              (message as any).isStreaming
            "
            :agent-name="currentAgent?.name"
            @retry="$emit('retry-message')"
            @regenerate="(id) => $emit('regenerate-from', id)"
            @copy="() => {}"
            @delete="() => {}"
          />
        </div>
      </div>

      <!-- 正在输入指示器 -->
      <div v-if="isTyping && !isStreaming" class="p-4">
        <div class="flex items-center space-x-3">
          <div
            class="w-8 h-8 rounded-full bg-gradient-to-br from-purple-500 to-pink-500 flex items-center justify-center text-white font-medium text-sm"
          >
            {{ currentAgent?.name?.charAt(0)?.toUpperCase() || "A" }}
          </div>
          <div class="flex items-center space-x-2">
            <div class="flex space-x-1">
              <div
                class="w-2 h-2 bg-gray-400 rounded-full animate-bounce"
                style="animation-delay: 0ms"
              ></div>
              <div
                class="w-2 h-2 bg-gray-400 rounded-full animate-bounce"
                style="animation-delay: 150ms"
              ></div>
              <div
                class="w-2 h-2 bg-gray-400 rounded-full animate-bounce"
                style="animation-delay: 300ms"
              ></div>
            </div>
            <span class="text-sm text-gray-500">{{
              t("agent.chat.agentTyping")
            }}</span>
          </div>
        </div>
      </div>

      <!-- 底部哨兵 -->
      <div ref="bottomSentinel" aria-hidden="true" class="h-px"></div>

      <!-- 回到底部按钮 -->
      <div class="sticky bottom-4 z-10">
        <transition name="fade">
          <button
            v-if="!isAtBottom"
            class="ml-auto mr-4 flex items-center gap-2 rounded-full shadow-lg px-3 py-2 bg-white border border-gray-200 hover:bg-gray-50 active:scale-95 transition pointer-events-auto"
            @click="jumpToBottom"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
              <path
                d="M6 9l6 6 6-6"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
            <span class="text-sm text-gray-700">
              {{
                unreadCount > 0
                  ? t("common.newMessages", { count: unreadCount })
                  : t("common.scrollToBottom")
              }}
            </span>
          </button>
        </transition>
      </div>
    </div>

    <!-- 输入区 -->
    <div class="flex-shrink-0 border-t border-gray-200 bg-white">
      <div class="p-4">
        <div class="mx-auto w-full max-w-screen-lg px-4 space-y-2">
          <!-- 第 1 行：输入行 -->
          <div class="flex items-center gap-2">
            <div class="relative flex-1">
              <!-- 左侧 + 下拉 -->
              <div class="absolute left-2 top-1/2 -translate-y-1/2 z-20">
                <UDropdownMenu
                  :items="[
                    [
                      {
                        label: '上传文件',
                        icon: 'i-heroicons-document-plus',
                        click: () => onUploadFile(),
                      },
                      {
                        label: '上传图片',
                        icon: 'i-heroicons-photo',
                        click: () => onUploadImage(),
                      },
                    ],
                    [
                      {
                        label: '默认模式',
                        icon: 'i-heroicons-sparkles',
                        click: () => selectMode('default'),
                        disabled: chatMode === 'default',
                      },
                      {
                        label: '代码模式',
                        icon: 'i-heroicons-code-bracket',
                        click: () => selectMode('code'),
                        disabled: chatMode === 'code',
                      },
                      {
                        label: '多模态模式',
                        icon: 'i-heroicons-photo',
                        click: () => selectMode('vision'),
                        disabled: chatMode === 'vision',
                      },
                    ],
                  ]"
                  :popper="{ placement: 'top-start' }"
                >
                  <UButton
                    variant="ghost"
                    size="sm"
                    aria-label="添加"
                    class="w-8 h-8 text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800"
                    :disabled="!canSendMessage || isStreaming"
                    :ui="{
                      base: 'inline-flex items-center justify-center p-0 gap-0 rounded-full',
                    }"
                  >
                    <UIcon name="i-heroicons-plus" class="w-4 h-4" />
                  </UButton>
                </UDropdownMenu>
              </div>

              <!-- 文本域 -->
              <textarea
                ref="inputRef"
                v-model="messageInput"
                :placeholder="
                  canSendMessage
                    ? t('agent.chat.inputPlaceholder')
                    : t('agent.chat.disconnectedPlaceholder')
                "
                :disabled="!canSendMessage || isStreaming"
                class="w-full resize-none border border-gray-300 dark:border-gray-600 rounded-xl pl-12 pr-24 px-4 py-3 leading-6 focus:ring-2 focus:ring-blue-500 focus:border-transparent disabled:bg-gray-100 disabled:cursor-not-allowed dark:bg-gray-900 dark:text-white"
                rows="1"
                style="min-height: 44px; max-height: 160px"
                autocomplete="off"
                autocorrect="off"
                autocapitalize="off"
                spellcheck="false"
                @input="adjustTextareaHeight"
                @keydown="handleKeydown"
                @compositionstart="isComposing = true"
                @compositionend="isComposing = false"
                @paste="handlePaste"
              />

              <!-- 右侧：麦克风 & 发送/停止 -->
              <div
                class="absolute right-2 top-1/2 -translate-y-1/2 flex items-center gap-1 z-20"
              >
                <UButton
                  variant="ghost"
                  size="sm"
                  aria-label="录音"
                  class="w-8 h-8 transition-colors"
                  :class="
                    isRecording
                      ? 'text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20'
                      : 'text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800'
                  "
                  :disabled="!canSendMessage || isStreaming"
                  @click="toggleRecording"
                  :ui="{
                    base: 'inline-flex items-center justify-center p-0 gap-0 rounded-full',
                  }"
                >
                  <UIcon
                    :name="
                      isRecording
                        ? 'i-heroicons-stop'
                        : 'i-heroicons-microphone'
                    "
                    class="w-4 h-4"
                  />
                </UButton>
                <UButton
                  size="sm"
                  :aria-label="isStreaming ? '停止生成' : '发送'"
                  class="w-8 h-8 transition-all"
                  :class="
                    isStreaming
                      ? 'bg-red-500 hover:bg-red-600 text-white'
                      : messageInput.trim() && canSendMessage
                        ? 'bg-green-500 hover:bg-green-600 text-white'
                        : 'bg-gray-200 dark:bg-gray-700 text-gray-400'
                  "
                  :disabled="
                    !canSendMessage || (!messageInput.trim() && !isStreaming)
                  "
                  @click="onSendClick"
                  :ui="{
                    base: 'inline-flex items-center justify-center p-0 gap-0 rounded-full',
                  }"
                >
                  <UIcon
                    :name="
                      isStreaming
                        ? 'i-heroicons-stop'
                        : 'i-heroicons-paper-airplane'
                    "
                    class="w-4 h-4"
                  />
                </UButton>
              </div>
            </div>

            <!-- 右侧：清空按钮 -->
            <div class="hidden sm:flex self-center">
              <UButton
                variant="ghost"
                size="sm"
                aria-label="清空"
                class="w-10 h-10 hover:bg-gray-100 dark:hover:bg-gray-800 disabled:opacity-50 disabled:cursor-not-allowed"
                :disabled="messages.length === 0"
                @click="$emit('clear-messages')"
                :ui="{
                  base: 'inline-flex items-center justify-center p-0 gap-0 rounded-full',
                }"
              >
                <UIcon name="i-heroicons-trash" class="w-5 h-5" />
              </UButton>
            </div>
          </div>

          <!-- 第 2 行：底部提示 -->
          <div class="flex items-center justify-between text-xs text-gray-500">
            <div
              v-if="currentAgent"
              class="flex items-center flex-wrap gap-x-2 gap-y-1"
            >
              <span>{{ t("agent.chat.model") }}: {{ currentAgent.model }}</span>
              <span>•</span>
              <span
                >{{ t("agent.chat.temperature") }}:
                {{ currentAgent.temperature }}</span
              >
              <span>•</span>
              <span
                >模式：{{
                  modeOptions.find((m) => m.value === chatMode)?.label || "默认"
                }}</span
              >
            </div>
            <div v-else></div>

            <div class="hidden sm:flex items-center gap-4">
              <span>{{ t("agent.chat.ctrlEnterToSend") }}</span>
              <span>{{ t("agent.chat.enterNewLine") }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- === Debug HUD（调试完删除） === -->
  <!-- <div
    v-if="true"
    class="fixed bottom-3 right-3 text-xs bg-black/70 text-white rounded px-3 py-2 space-y-1 z-50"
  >
    <div>msgs: {{ messages.length }}</div>
    <div v-if="messages.length">
      <div>last.id: {{ messages[messages.length - 1].id }}</div>
      <div>role: {{ messages[messages.length - 1].role }}</div>
      <div>
        len:
        {{
          typeof messages[messages.length - 1].content === "string"
            ? (messages[messages.length - 1].content as string).length
            : -1
        }}
      </div>
      <div>
        isStreaming:
        {{ (messages[messages.length - 1] as any).isStreaming ? "T" : "F" }}
      </div>
      <div>
        isThinking:
        {{ (messages[messages.length - 1] as any).isThinking ? "T" : "F" }}
      </div>
      <div>
        done: {{ (messages[messages.length - 1] as any).done ? "T" : "F" }}
      </div>
    </div>
  </div> -->
</template>

<style scoped>
.overflow-y-auto::-webkit-scrollbar {
  width: 6px;
}
.overflow-y-auto::-webkit-scrollbar-track {
  background: #f1f1f1;
}
.overflow-y-auto::-webkit-scrollbar-thumb {
  background: #c1c1c1;
  border-radius: 3px;
}
.overflow-y-auto::-webkit-scrollbar-thumb:hover {
  background: #a8a8a8;
}

.fade-enter-active,
.fade-leave-active {
  transition:
    opacity 0.15s ease,
    transform 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>
