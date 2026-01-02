<script setup lang="ts">
import { computed, ref } from "vue";

const props = defineProps<{
  code?: string;
  language?: string | null;
  filename?: string | null;
  highlights?: unknown[];
  meta?: string | null;
  class?: string | null;
}>();

const copied = ref(false);

const label = computed(() => {
  if (props.filename) return String(props.filename);
  if (props.language) return String(props.language).toUpperCase();
  return "CODE";
});

const onCopy = async () => {
  const text = String(props.code ?? "");
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
    copied.value = true;
    setTimeout(() => (copied.value = false), 1200);
  } catch {}
};
</script>

<template>
  <div class="px-codeblock">
    <div class="px-codeblock__header">
      <span class="px-codeblock__lang">{{ label }}</span>
      <button type="button" class="px-codeblock__copy" @click="onCopy">
        {{ copied ? "已复制" : "复制" }}
      </button>
    </div>
    <pre :class="['px-codeblock__pre', props.class]"><slot /></pre>
  </div>
</template>

<style scoped>
.px-codeblock {
  border-radius: 0.75rem;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: #0b1220;
  margin: 0.75rem 0;
}
.px-codeblock__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0.75rem;
  background: rgba(255, 255, 255, 0.06);
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}
.px-codeblock__lang {
  font-size: 0.75rem;
  letter-spacing: 0.02em;
  color: rgba(229, 231, 235, 0.9);
}
.px-codeblock__copy {
  font-size: 0.75rem;
  color: rgba(229, 231, 235, 0.9);
  padding: 0.25rem 0.5rem;
  border-radius: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.04);
}
.px-codeblock__copy:hover {
  background: rgba(255, 255, 255, 0.08);
}
.px-codeblock__pre {
  margin: 0;
  padding: 0.75rem;
  overflow-x: auto;
  background: transparent;
}
.px-codeblock__pre :deep(code) {
  display: block;
  white-space: pre;
  color: #e5e7eb;
  font-size: 0.875rem;
  line-height: 1.45;
  background: transparent;
  padding: 0;
}

/* Light mode：保持 ChatGPT 一样代码块仍然深色，但边框更柔和 */
:global(html:not(.dark)) .px-codeblock {
  border-color: rgba(17, 24, 39, 0.12);
}
</style>

