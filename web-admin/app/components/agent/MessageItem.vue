<script setup lang="ts">
import type { ChatMessage } from "~/types/message";
import type { MessageContent } from "~/types/message";
import type { DeepReadonly } from "vue";
import { computed, watch, onBeforeUnmount } from "vue";
import { useI18n } from "#imports";
import { useThinkParser } from "~/composables/agent/useThinkParser";
import { useMessageTypewriter } from "~/composables/agent/useTypewriter";
import ThinkBlock from "~/components/agent/ThinkBlock.vue";
import { MESSAGE_TYPES } from "~/types/message";

declare global {
  interface Window {
    open(url?: string | URL, target?: string, features?: string): Window | null;
  }
}

const props = defineProps<{
  message: ChatMessage | DeepReadonly<ChatMessage>;
  isStreaming?: boolean;
  agentName?: string;
}>();

const emit = defineEmits<{
  (e: "retry"): void;
  (e: "copy", content: string): void;
  (e: "delete"): void;
}>();

const { t } = useI18n();

// 原始完整文本
const normalizedRawContent = computed(() => {
  const c = (props.message as any)?.content;
  if (typeof c === "string") return c;
  // ✅ 单对象（MessageContent）
  if (c && typeof c === "object" && !Array.isArray(c)) {
    const t = c.type;
    const d = c.data ?? {};
    if (t === MESSAGE_TYPES.TEXT && d.text) return String(d.text);
    if (t === MESSAGE_TYPES.MARKDOWN && d.markdown) return String(d.markdown);
    if (t === MESSAGE_TYPES.CODE && d.code) return String(d.code);
    return "";
  }
  if (Array.isArray(c)) {
    return c
      .map((item: any) => {
        if (!item || !item.type) return "";
        if (item.type === MESSAGE_TYPES.TEXT && item.data?.text)
          return String(item.data.text);
        if (item.type === MESSAGE_TYPES.MARKDOWN && item.data?.markdown)
          return String(item.data.markdown);
        if (item.type === MESSAGE_TYPES.CODE && item.data?.code)
          return String(item.data.code);
        return "";
      })
      .join("\n")
      .trim();
  }
  return "";
});
const fullContentRef = normalizedRawContent;

// ====== 静态解析（用于回退：后端未提供 meta.think 时）======
const { parsedMessage } = useThinkParser(fullContentRef);

// 方便调试
watch(
  () => ({
    id: (props.message as any)?.id,
    role: props.message.role,
    len:
      typeof props.message.content === "string"
        ? (props.message.content as string).length
        : -1,
    isStreaming: (props.message as any)?.isStreaming,
    isThinking: (props.message as any)?.isThinking,
    done: (props.message as any)?.done,
    metaThink: (props.message as any)?.meta?.think,
  }),
  (v) => {
    // console.log("[MessageItem]", v);
  },
  { deep: false, immediate: true }
);

// ====== 打字机：仅用于正文主内容（已剥离 think）======
const streamMode = computed(
  () => (props.message as any)?.meta?.streamMode || "delta"
);
const shouldUseTypewriter = computed(
  () =>
    props.message.role === "assistant" &&
    !(props.message as any).isError &&
    !(props.message as any).isThinking &&
    ((props.isStreaming ?? false) || (props.message as any).isStreaming) &&
    streamMode.value === "delta" // 只有增量才打字
);

const typewriter = useMessageTypewriter({
  speed: 25,
  onComplete: () => {},
});

// “正在打字的可见文本”解析（兜底去 think）
const displayedParsed = useThinkParser(
  computed(() => typewriter?.displayedText?.value ?? "")
);

// 强制去除 <think>…</think> 兜底
const stripThink = (s: string) =>
  (s ?? "")
    .replace(/<think>[\s\S]*?<\/think>/gi, "") // 去完整块
    .replace(/<think[\s\S]*$/i, "") // 去未闭合尾巴
    .trim();

// 只在“明确完成”才 complete，避免过早掐断
watch(
  [
    () => normalizedRawContent.value, // ✅ 用规范化后的纯文本
    () => (props.message as any).isStreaming,
    () => (props.message as any).isThinking,
    () => (props.message as any).done,
  ],
  ([normText, isStreaming, isThinking, done]) => {
    const text = String(normText ?? "");
    // 流式且不处于思考阶段时，走打字机；否则直接设定文本
    if (
      (isStreaming || props.isStreaming) &&
      !isThinking &&
      props.message.role === "assistant" &&
      !(props.message as any).isError
    ) {
      typewriter.updateMessage(text, true);
    } else {
      typewriter.setText(text, false);
      if (done === true) typewriter.complete();
    }
  },
  { immediate: true }
);

onBeforeUnmount(() => {
  typewriter?.cleanup?.();
});

// ====== Think 渲染数据：优先 meta.think，回退 parser ======
const thinkMeta = computed(
  () =>
    (props.message as any)?.meta?.think ??
    (props.message as any)?.metadata?.think
);

const normalize = (s: string) =>
  String(s ?? "")
    .replace(/\s+/g, " ")
    .trim();

const lastCompleted = computed(() =>
  completedThinkBlocks.value.length > 0
    ? completedThinkBlocks.value[completedThinkBlocks.value.length - 1]
    : null
);

const shouldShowActiveThink = computed(() => {
  if (!hasActiveThink.value || !activeThinkNonEmpty.value) return false;
  if (!lastCompleted.value) return true;
  return (
    normalize(activeThinkContent.value) !==
    normalize(lastCompleted.value.content)
  );
});

// 提取“非空”的 <think>…</think> 段（去除空白后长度>0 才算）
const nonEmptyThinkSegmentsInRaw = computed(() => {
  const raw =
    typewriter?.displayedText?.value &&
    ((props.message as any)?.isStreaming || props.isStreaming)
      ? String(typewriter.displayedText.value)
      : normalizedRawContent.value;
  const matches = Array.from(raw.matchAll(/<think>([\s\S]*?)<\/think>/gi)).map(
    (m) =>
      String(m[1] ?? "")
        .replace(/\s+/g, " ")
        .trim()
  );
  return matches.filter((seg) => seg.length > 0);
});
// 只有当“原文中存在至少一个非空的 <think> 段”时才允许显示
const hasNonEmptyThinkTag = computed(
  () => nonEmptyThinkSegmentsInRaw.value.length > 0
);

const completedThinkBlocks = computed(() => {
  // 1) 优先使用后端 meta blocks
  const blocksFromMeta =
    thinkMeta.value?.blocks
      ?.map((b: any, i: number) => ({
        content: String(b?.content ?? ""),
        index: typeof b?.index === "number" ? b.index : i,
      }))
      .filter((b) => b.content.replace(/\s+/g, " ").trim().length > 0) ?? null;

  // console.log("[MessageItem] blocksFromMeta", blocksFromMeta);
  const dedupe = (blocks: any[]) => {
    const seen = new Set<string>();
    const result: any[] = [];
    for (const b of blocks) {
      const key = normalize(b.content);
      if (!key || seen.has(key)) continue;
      seen.add(key);
      result.push(b);
    }
    return result;
  };

  if (blocksFromMeta && blocksFromMeta.length > 0) {
    // console.log("[MessageItem] blocksFromMeta", blocksFromMeta);
    return dedupe(blocksFromMeta);
  }

  // 2) 回退解析：仅当正文包含 <think>…</think> 时
  if (!hasNonEmptyThinkTag.value) return [];
  // console.log("[MessageItem] parsedMessage", parsedMessage.value);
  const parsed = parsedMessage.value.thinkBlocks || [];
  return dedupe(
    parsed.filter(
      (b: any) =>
        String(b?.content ?? "")
          .replace(/\s+/g, " ")
          .trim().length > 0
    )
  );
});

const activeThinkContent = computed(() => thinkMeta.value?.current ?? "");
// 兜底：在流式中 && 有 current 文本 时视为“有活动块”
const hasActiveThink = computed(() => {
  const flagFromMeta = !!thinkMeta.value?.hasActiveThink;
  const streaming = (props.message as any)?.isStreaming || !!props.isStreaming;
  const hasCurrent = String(activeThinkContent.value).trim().length > 0;
  return flagFromMeta || (streaming && hasCurrent);
});

const activeThinkNonEmpty = computed(() => {
  const s = String(activeThinkContent.value || "")
    .replace(/\s+/g, " ")
    .trim();
  return hasActiveThink.value && s.length > 0;
});

// 只有“原文中存在非空 <think> 段”才允许显示（仍遵循你之前的约束：以原文标签为准）
const showThink = computed(
  () => completedThinkBlocks.value.length > 0 || shouldShowActiveThink.value
);

// ====== 主体渲染内容（纯主内容，剥离 think；流式时走打字机）======
const processedContent = computed<MessageContent[]>(() => {
  const usingTyping =
    props.message.role === "assistant" &&
    !(props.message as any).isError &&
    !(props.message as any).isThinking &&
    ((props.isStreaming ?? false) || (props.message as any).isStreaming);

  const visible = usingTyping
    ? (typewriter?.displayedText?.value ?? "")
    : normalizedRawContent.value; // ✅ 统一来源

  const text = stripThink(visible);
  return text ? [{ type: MESSAGE_TYPES.TEXT, data: { text } }] : [];
});

// 工具函数 & 展示辅助
const copyToClipboard = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text);
    emit("copy", text);
  } catch (err) {
    console.error("复制失败:", err);
  }
};
const formatFileSize = (bytes: number) => {
  if (bytes === 0) return "0 Bytes";
  const k = 1024,
    sizes = ["Bytes", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
};
const formatTime = (date: Date | string | number) => {
  try {
    const d = new Date(date);
    if (isNaN(d.getTime())) return "无效时间";
    return new Intl.DateTimeFormat("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
    }).format(d);
  } catch {
    return "无效时间";
  }
};
const getLanguageDisplayName = (lang: string) => {
  const map: Record<string, string> = {
    javascript: "JavaScript",
    typescript: "TypeScript",
    python: "Python",
    java: "Java",
    cpp: "C++",
    csharp: "C#",
    php: "PHP",
    go: "Go",
    rust: "Rust",
    sql: "SQL",
    html: "HTML",
    css: "CSS",
    json: "JSON",
    yaml: "YAML",
    xml: "XML",
    bash: "Bash",
    shell: "Shell",
  };
  return map[lang.toLowerCase()] || lang.toUpperCase();
};
const openExternalLink = (url: string) => {
  if (typeof window !== "undefined") window.open(url, "_blank");
};
const downloadFile = (url: string, downloadUrl?: string) => {
  if (typeof window !== "undefined") window.open(downloadUrl || url, "_blank");
};

// 简单 Markdown 渲染（保持你的原逻辑）
const renderMarkdown = (markdown: string) => {
  let html = markdown;

  // 1) 标题：允许最多 3 个空格缩进 & 允许 # 号后无空格
  //    例：###1. 生死观  或  ### 1. 生死观  都能匹配
  html = html.replace(
    /^\s{0,3}(#{1,6})\s*(.*)$/gm,
    (_m, hashes: string, text: string) => {
      const level = Math.min(hashes.length, 6);
      return `<h${level}>${text.trim()}</h${level}>`;
    }
  );

  // 2) 粗体 / 斜体 / 行内代码 / 链接（保持你的原逻辑）
  html = html.replace(/\*\*(.*?)\*\*/g, "<strong>$1</strong>");
  html = html.replace(/\*(.*?)\*/g, "<em>$1</em>");
  html = html.replace(/`([^`]+)`/g, "<code>$1</code>");
  html = html.replace(
    /\[([^\]]+)\]\(([^)]+)\)/g,
    '<a href="$2" target="_blank" class="text-blue-600 hover:underline">$1</a>'
  );

  // 3) 引用（允许缩进）
  html = html.replace(/^\s*>\s+(.*)$/gm, "<blockquote>$1</blockquote>");

  // 4) 表格（保持你的原逻辑）
  const tableRegex = /\|(.+)\|\n\|[-\s|:]+\|\n((?:\|.+\|\n?)*)/g;
  html = html.replace(tableRegex, (match, header, rows) => {
    const headerCells = String(header)
      .split("|")
      .map((c: string) => c.trim())
      .filter(Boolean);
    const headerRow =
      "<tr>" +
      headerCells.map((c: string) => `<th>${c}</th>`).join("") +
      "</tr>";
    const bodyRows = String(rows)
      .trim()
      .split("\n")
      .map((row: string) => {
        const cells = row
          .split("|")
          .map((c: string) => c.trim())
          .filter(Boolean);
        return (
          "<tr>" + cells.map((c: string) => `<td>${c}</td>`).join("") + "</tr>"
        );
      })
      .join("");
    return `<table class="border-collapse border border-gray-300"><thead>${headerRow}</thead><tbody>${bodyRows}</tbody></table>`;
  });

  // 5) 无序列表：允许缩进（- * +），先每行各自包一层 <ul>，再合并相邻的 <ul>
  html = html.replace(/^\s*[-*+]\s+(.*)$/gm, "<ul><li>$1</li></ul>");
  html = html.replace(/<\/ul>\s*<ul>/g, "");

  // 6) 有序列表：允许缩进（1. 2. ...），同样先包 <ol> 再合并
  html = html.replace(/^\s*\d+\.\s+(.*)$/gm, "<ol><li>$1</li></ol>");
  html = html.replace(/<\/ol>\s*<ol>/g, "");

  // 7) 换行（放到最后）
  html = html.replace(/\n/g, "<br>");

  return html;
};
</script>

<template>
  <div class="p-4 hover:bg-gray-50 transition-colors group">
    <div class="flex space-x-3">
      <!-- 头像 -->
      <div class="flex-shrink-0">
        <div
          v-if="message.role === 'user'"
          class="w-8 h-8 rounded-full bg-blue-500 flex items-center justify-center text-white font-medium text-sm"
        >
          U
        </div>
        <div
          v-else-if="message.role === 'assistant'"
          class="w-8 h-8 rounded-full bg-gradient-to-br from-purple-500 to-pink-500 flex items-center justify-center text-white font-medium text-sm"
        >
          {{ agentName?.charAt(0)?.toUpperCase() || "A" }}
        </div>
        <div
          v-else
          class="w-8 h-8 rounded-full bg-gray-500 flex items-center justify-center text-white font-medium text-sm"
        >
          <UIcon name="i-heroicons-cog-6-tooth" class="w-4 h-4" />
        </div>
      </div>

      <!-- 内容 -->
      <div class="flex-1 min-w-0">
        <!-- 头部 -->
        <div class="flex items-center space-x-2 mb-2">
          <span class="font-medium text-gray-900">
            {{
              message.role === "user"
                ? t("agent.chat.you")
                : message.role === "assistant"
                  ? agentName || t("agent.chat.assistant")
                  : t("agent.chat.system")
            }}
          </span>
          <span class="text-xs text-gray-500">{{
            formatTime(message.timestamp)
          }}</span>
          <div
            v-if="(message as any).isStreaming || isStreaming"
            class="flex items-center space-x-1"
          >
            <div class="w-1 h-1 bg-blue-500 rounded-full animate-pulse"></div>
            <span class="text-xs text-blue-500">{{
              t("agent.chat.generating")
            }}</span>
          </div>
        </div>

        <!-- “正在思考…” 提示：仅在没有可显示的 ThinkBlock 时出现 -->
        <div
          v-if="(message as any).isThinking && !showThink"
          class="flex items-center space-x-3 py-3"
        >
          <div class="flex space-x-1 items-center">
            <div class="w-2 h-2 bg-gray-400 rounded-full thinking-dot"></div>
            <div class="w-2 h-2 bg-gray-400 rounded-full thinking-dot"></div>
            <div class="w-2 h-2 bg-gray-400 rounded-full thinking-dot"></div>
          </div>
          <span class="text-sm text-gray-500 italic">
            {{ agentName || t("agent.chat.assistant") }} 正在思考...
          </span>
        </div>

        <!-- Think 区块（优先使用 meta.think；无则回退 parser） -->
        <!-- Think 区块（只有正文包含 <think> 才出现） -->
        <div v-if="showThink" class="space-y-2 mb-4">
          <!-- 已完成的 think 块：保持默认，是否展开你随意 -->
          <ThinkBlock
            v-for="(b, i) in completedThinkBlocks"
            :key="`think-completed-${i}`"
            :content="b.content"
            :index="b.index"
            :is-streaming="false"
            :default-expanded="false"
          />

          <!-- 正在进行的 think 块：默认收起 + 标题“思考中...” -->
          <ThinkBlock
            v-if="shouldShowActiveThink"
            :content="
              ((message as any).meta?.think ?? (message as any).metadata?.think)
                ?.current
            "
            :index="completedThinkBlocks.length"
            :is-streaming="true"
            :default-expanded="false"
            :label="'思考中...'"
            :auto-expand-on-streaming="false"
          />
        </div>

        <!-- 主体（正文） -->
        <div class="space-y-3">
          <template v-for="(content, index) in processedContent" :key="index">
            <!-- 文本（保持原样式容器，只把插值改成 v-html） -->
            <div
              v-if="content.type === MESSAGE_TYPES.TEXT"
              class="prose prose-sm max-w-none"
            >
              <div
                class="text-gray-800 whitespace-pre-wrap markdown-content"
                v-html="renderMarkdown(content.data.text)"
              ></div>
              <span
                v-if="
                  (message as any).role === 'assistant' &&
                  ((message as any).isStreaming || isStreaming) &&
                  !(message as any).isThinking
                "
                class="inline-block w-0.5 h-4 bg-blue-500 ml-0.5 animate-pulse"
                style="animation: blink 1s infinite"
              />
            </div>

            <!-- Markdown -->
            <div
              v-else-if="content.type === MESSAGE_TYPES.MARKDOWN"
              class="prose prose-sm max-w-none"
            >
              <div class="bg-gray-50 rounded-lg p-4 border">
                <div class="flex items-center justify-between mb-2">
                  <span class="text-xs font-medium text-gray-600 uppercase"
                    >Markdown</span
                  >
                  <UButton
                    size="xs"
                    variant="ghost"
                    icon="i-heroicons-clipboard"
                    @click="copyToClipboard(content.data.markdown)"
                  />
                </div>
                <div class="markdown-content">
                  <div v-html="renderMarkdown(content.data.markdown)"></div>
                </div>
              </div>
            </div>

            <!-- 代码 -->
            <div
              v-else-if="content.type === MESSAGE_TYPES.CODE"
              class="bg-gray-900 rounded-lg overflow-hidden"
            >
              <div
                class="flex items-center justify-between px-4 py-2 bg-gray-800 border-b border-gray-700"
              >
                <div class="flex items-center space-x-2">
                  <UIcon
                    name="i-heroicons-code-bracket"
                    class="w-4 h-4 text-gray-400"
                  />
                  <span class="text-sm font-medium text-gray-300">
                    {{ getLanguageDisplayName(content.data.language) }}
                  </span>
                  <span
                    v-if="content.data.filename"
                    class="text-xs text-gray-500"
                  >
                    {{ content.data.filename }}
                  </span>
                </div>
                <UButton
                  size="xs"
                  variant="ghost"
                  icon="i-heroicons-clipboard"
                  class="text-gray-400 hover:text-white"
                  @click="copyToClipboard(content.data.code)"
                />
              </div>
              <pre
                class="p-4 text-sm text-gray-100 overflow-x-auto"
              ><code>{{ content.data.code }}</code></pre>
            </div>

            <!-- 图片 -->
            <div
              v-else-if="content.type === MESSAGE_TYPES.IMAGE"
              class="space-y-2"
            >
              <div
                class="relative inline-block rounded-lg overflow-hidden border border-gray-200"
              >
                <img
                  :src="content.data.url"
                  :alt="content.data.alt || '图片'"
                  :style="{
                    maxWidth: content.data.width
                      ? `${content.data.width}px`
                      : '400px',
                    maxHeight: content.data.height
                      ? `${content.data.height}px`
                      : '300px',
                  }"
                  class="object-cover"
                />
                <div class="absolute top-2 right-2">
                  <UButton
                    size="xs"
                    variant="solid"
                    color="neutral"
                    icon="i-heroicons-arrow-top-right-on-square"
                    @click="() => openExternalLink(content.data.url)"
                  />
                </div>
              </div>
              <p v-if="content.data.caption" class="text-sm text-gray-600">
                {{ content.data.caption }}
              </p>
            </div>

            <!-- 视频 -->
            <div
              v-else-if="content.type === MESSAGE_TYPES.VIDEO"
              class="space-y-2"
            >
              <div
                class="relative rounded-lg overflow-hidden border border-gray-200 bg-black"
              >
                <video
                  :src="content.data.url"
                  :poster="content.data.poster"
                  controls
                  class="w-full max-w-md"
                  style="max-height: 300px"
                >
                  您的浏览器不支持视频播放
                </video>
              </div>
              <div
                class="flex items-center justify-between text-sm text-gray-600"
              >
                <span v-if="content.data.caption">{{
                  content.data.caption
                }}</span>
                <span v-if="content.data.duration" class="text-xs">
                  {{ Math.floor(content.data.duration / 60) }}:{{
                    String(content.data.duration % 60).padStart(2, "0")
                  }}
                </span>
              </div>
            </div>

            <!-- 卡片 -->
            <div
              v-else-if="content.type === MESSAGE_TYPES.CARD"
              class="max-w-sm border border-gray-200 rounded-lg overflow-hidden bg-white shadow-sm"
            >
              <div v-if="content.data.image" class="aspect-[4/3] bg-gray-100">
                <img
                  :src="content.data.image"
                  :alt="content.data.title"
                  class="w-full h-full object-cover"
                />
              </div>
              <div class="p-3">
                <h3 class="font-semibold text-gray-900 mb-1 text-sm">
                  {{ content.data.title }}
                </h3>
                <p
                  v-if="content.data.description"
                  class="text-gray-600 text-xs mb-2 line-clamp-2"
                >
                  {{ content.data.description }}
                </p>
                <div v-if="content.data.metadata" class="space-y-0.5 mb-2">
                  <div
                    v-for="(value, key) in content.data.metadata"
                    :key="key"
                    class="flex justify-between text-xs text-gray-500"
                  >
                    <span>{{ key }}:</span>
                    <span>{{ value }}</span>
                  </div>
                </div>
                <div v-if="content.data.actions" class="flex space-x-1">
                  <UButton
                    v-for="action in content.data.actions"
                    :key="action.label"
                    :variant="action.variant || 'outline'"
                    size="xs"
                    @click="console.log('Action:', action.action)"
                  >
                    {{ action.label }}
                  </UButton>
                </div>
              </div>
            </div>

            <!-- 文件 -->
            <div
              v-else-if="content.type === MESSAGE_TYPES.FILE"
              class="border border-gray-200 rounded-lg p-4 bg-gray-50"
            >
              <div class="flex items-center space-x-3">
                <div class="flex-shrink-0">
                  <UIcon
                    name="i-heroicons-document"
                    class="w-8 h-8 text-gray-500"
                  />
                </div>
                <div class="flex-1 min-w-0">
                  <p class="font-medium text-gray-900 truncate">
                    {{ content.data.name }}
                  </p>
                  <p class="text-sm text-gray-500">
                    {{ content.data.type }} •
                    {{ formatFileSize(content.data.size) }}
                  </p>
                </div>
                <div class="flex-shrink-0">
                  <UButton
                    size="sm"
                    variant="outline"
                    icon="i-heroicons-arrow-down-tray"
                    @click="
                      () =>
                        downloadFile(content.data.url, content.data.downloadUrl)
                    "
                  >
                    下载
                  </UButton>
                </div>
              </div>
            </div>

            <!-- 系统消息 -->
            <div
              v-else-if="content.type === MESSAGE_TYPES.SYSTEM"
              class="rounded-lg p-3"
              :class="{
                'bg-blue-50 border border-blue-200':
                  content.data.level === 'info',
                'bg-yellow-50 border border-yellow-200':
                  content.data.level === 'warning',
                'bg-red-50 border border-red-200':
                  content.data.level === 'error',
                'bg-green-50 border border-green-200':
                  content.data.level === 'success',
              }"
            >
              <div class="flex items-center space-x-2">
                <UIcon
                  :name="
                    (
                      {
                        info: 'i-heroicons-information-circle',
                        warning: 'i-heroicons-exclamation-triangle',
                        error: 'i-heroicons-x-circle',
                        success: 'i-heroicons-check-circle',
                      } as Record<string, string>
                    )[content.data.level] || 'i-heroicons-information-circle'
                  "
                  :class="{
                    'text-blue-500': content.data.level === 'info',
                    'text-yellow-500': content.data.level === 'warning',
                    'text-red-500': content.data.level === 'error',
                    'text-green-500': content.data.level === 'success',
                  }"
                  class="w-5 h-5"
                />
                <span
                  class="text-sm font-medium"
                  :class="{
                    'text-blue-800': content.data.level === 'info',
                    'text-yellow-800': content.data.level === 'warning',
                    'text-red-800': content.data.level === 'error',
                    'text-green-800': content.data.level === 'success',
                  }"
                >
                  {{ content.data.message }}
                </span>
              </div>
            </div>
          </template>
        </div>

        <!-- 操作区 -->
        <div
          class="flex items-center space-x-2 mt-3 opacity-0 group-hover:opacity-100 transition-opacity"
        >
          <UButton
            v-if="message.role === 'assistant'"
            size="xs"
            variant="ghost"
            icon="i-heroicons-arrow-path"
            @click="emit('retry')"
            >重试</UButton
          >
          <UButton
            size="xs"
            variant="ghost"
            icon="i-heroicons-clipboard"
            @click="
              copyToClipboard(
                typeof message.content === 'string'
                  ? message.content
                  : JSON.stringify(message.content)
              )
            "
            >复制</UButton
          >
          <UButton
            size="xs"
            variant="ghost"
            icon="i-heroicons-trash"
            color="error"
            @click="emit('delete')"
            >删除</UButton
          >
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.group:hover .group-hover\:opacity-100 {
  opacity: 1;
}

.markdown-content {
  color: #1f2937;
}
.markdown-content h1,
.markdown-content h2,
.markdown-content h3,
.markdown-content h4,
.markdown-content h5,
.markdown-content h6 {
  font-weight: 600;
  color: #111827;
  margin-top: 1rem;
  margin-bottom: 0.5rem;
}
.markdown-content p {
  margin-bottom: 0.75rem;
}
.markdown-content ul,
.markdown-content ol {
  margin-left: 1rem;
  margin-bottom: 0.75rem;
}
.markdown-content li {
  margin-bottom: 0.25rem;
}
.markdown-content code {
  background-color: #f3f4f6;
  color: #1f2937;
  padding: 0.125rem 0.25rem;
  border-radius: 0.25rem;
  font-size: 0.875rem;
  font-family: "Monaco", "Menlo", "Ubuntu Mono", monospace;
}
.markdown-content pre {
  background-color: #f3f4f6;
  padding: 0.75rem;
  border-radius: 0.5rem;
  overflow-x: auto;
}
.markdown-content blockquote {
  border-left: 4px solid #d1d5db;
  padding-left: 1rem;
  font-style: italic;
  color: #4b5563;
}

/* 思考动画 */
@keyframes thinking-bounce {
  0%,
  60%,
  100% {
    transform: translateY(0);
  }
  30% {
    transform: translateY(-8px);
  }
}
.thinking-dot {
  animation: thinking-bounce 1.4s infinite ease-in-out;
}
.thinking-dot:nth-child(1) {
  animation-delay: 0ms;
}
.thinking-dot:nth-child(2) {
  animation-delay: 200ms;
}
.thinking-dot:nth-child(3) {
  animation-delay: 400ms;
}

/* 打字机光标动画 */
@keyframes blink {
  0%,
  50% {
    opacity: 1;
  }
  51%,
  100% {
    opacity: 0;
  }
}
</style>
