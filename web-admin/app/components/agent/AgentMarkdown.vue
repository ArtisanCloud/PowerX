<script setup lang="ts">
import { parseMarkdown, createCachedParser } from "@nuxtjs/mdc/runtime";
import { computed, onBeforeUnmount, ref, watch } from "vue";

const props = withDefaults(
  defineProps<{
    source: string;
    streaming?: boolean;
  }>(),
  {
    source: "",
    streaming: false,
  }
);

type Parsed = {
  body: any;
  data?: any;
};

const collapseInlineCodeLines = (md: string) => {
  const src = String(md ?? "").replace(/\r\n/g, "\n");
  const lines = src.split("\n");
  const out: string[] = [];
  let i = 0;
  while (i < lines.length) {
    const m = lines[i].match(/^\s*`([^`]+)`\s*$/);
    if (!m) {
      out.push(lines[i]);
      i++;
      continue;
    }
    const run: string[] = [m[1]];
    let j = i + 1;
    while (j < lines.length) {
      const mj = lines[j].match(/^\s*`([^`]+)`\s*$/);
      if (!mj) break;
      run.push(mj[1]);
      j++;
    }
    if (run.length >= 2) {
      out.push("```");
      out.push(...run);
      out.push("```");
    } else {
      out.push(lines[i]);
    }
    i = j;
  }
  return out.join("\n");
};

const normalizedSource = computed(() => collapseInlineCodeLines(props.source));

const parsed = ref<Parsed | null>(null);
const parseErr = ref<string>("");

// 流式时用增量 parser（只要内容是 append，就能更省开销）
const incrementalParse = createCachedParser({});

let timer: ReturnType<typeof setTimeout> | null = null;
const clearTimer = () => {
  if (timer) clearTimeout(timer);
  timer = null;
};
onBeforeUnmount(() => clearTimer());

const doParse = async () => {
  const src = normalizedSource.value || "";
  if (!src.trim()) {
    parsed.value = null;
    parseErr.value = "";
    return;
  }
  try {
    parseErr.value = "";
    const res = props.streaming
      ? await incrementalParse(src)
      : await parseMarkdown(src, {
          // 让 @nuxtjs/mdc 走它自带的 rehype-highlight/shiki 配置
          //（由 Nuxt Content/配置决定）
        });
    parsed.value = (res as any) || null;
  } catch (e) {
    parsed.value = null;
    parseErr.value = e instanceof Error ? e.message : String(e);
  }
};

watch(
  () => [normalizedSource.value, props.streaming] as const,
  async () => {
    clearTimer();
    // 流式时做轻微 debounce，避免每个 token 都触发一次 parse
    timer = setTimeout(() => void doParse(), props.streaming ? 120 : 0);
  },
  { immediate: true }
);
</script>

<template>
  <div class="px-md">
    <div v-if="parsed?.body" class="px-md__content">
      <MDCRenderer :body="parsed.body" :data="parsed.data" prose />
    </div>
    <pre v-else-if="parseErr" class="px-md__fallback">{{ source }}</pre>
  </div>
</template>

<style scoped>
.px-md {
  width: 100%;
}
.px-md__content {
  width: 100%;
  overflow-x: auto;
}
.px-md__content :deep(table) {
  width: 100%;
  min-width: 36rem;
  margin: 1rem 0;
  border-collapse: separate;
  border-spacing: 0;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, currentColor 18%, transparent);
  border-radius: 0.625rem;
}
.px-md__content :deep(h1),
.px-md__content :deep(h2),
.px-md__content :deep(h3) {
  margin: 1.25rem 0 0.625rem;
  font-weight: 650;
  line-height: 1.35;
}
.px-md__content :deep(h1) { font-size: 1.25rem; }
.px-md__content :deep(h2) { font-size: 1.125rem; }
.px-md__content :deep(h3) { font-size: 1rem; }
.px-md__content :deep(p) { margin: 0.5rem 0; }
.px-md__content :deep(ul),
.px-md__content :deep(ol) { margin: 0.5rem 0; padding-left: 1.5rem; }
.px-md__content :deep(li + li) { margin-top: 0.25rem; }
.px-md__content :deep(th),
.px-md__content :deep(td) {
  padding: 0.625rem 0.75rem;
  text-align: left;
  vertical-align: top;
  border-right: 1px solid color-mix(in srgb, currentColor 14%, transparent);
  border-bottom: 1px solid color-mix(in srgb, currentColor 14%, transparent);
}
.px-md__content :deep(th) {
  font-weight: 600;
  background: color-mix(in srgb, currentColor 8%, transparent);
}
.px-md__content :deep(th:last-child),
.px-md__content :deep(td:last-child) {
  border-right: 0;
}
.px-md__content :deep(tbody tr:last-child td) {
  border-bottom: 0;
}
.px-md__fallback {
  white-space: pre-wrap;
  word-break: break-word;
  padding: 0.75rem;
  border-radius: 0.75rem;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
}
</style>
