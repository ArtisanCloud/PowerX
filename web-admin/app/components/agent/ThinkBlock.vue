<template>
  <div class="think-block">
    <div
      class="think-header"
      @click="toggleExpanded"
      :class="{ expanded: isExpanded }"
    >
      <div class="think-icon">
        <Icon name="lucide:brain" class="w-4 h-4" />
      </div>
      <span class="think-label">
        <!-- 流式时显示“思考中...” + 动画点点 -->
        <template v-if="isStreaming">
          思考中
          <span class="dots">
            <span class="dot"></span>
            <span class="dot"></span>
            <span class="dot"></span>
          </span>
        </template>
        <template v-else>
          {{ index === 0 ? "思考过程" : `思考过程 ${index + 1}` }}
        </template>
      </span>
      <div class="think-toggle">
        <Icon
          :name="isExpanded ? 'lucide:chevron-up' : 'lucide:chevron-down'"
          class="w-4 h-4 transition-transform duration-200"
        />
      </div>
    </div>

    <Transition name="think-content">
      <div v-if="isExpanded" class="think-content">
        <div class="think-text" v-html="formattedContent"></div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from "vue";

interface Props {
  content: string;
  index?: number;
  isStreaming?: boolean; // = 正在思考
  defaultExpanded?: boolean; // 默认展开？思考中建议 false
}

const props = withDefaults(defineProps<Props>(), {
  index: 0,
  isStreaming: false,
  defaultExpanded: false,
});

const isExpanded = ref<boolean>(!!props.defaultExpanded); // 默认收起

const formattedContent = computed(() => {
  return (props.content || "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\n/g, "<br>")
    .replace(/\s{2,}/g, (m) => "&nbsp;".repeat(m.length));
});

const toggleExpanded = () => {
  isExpanded.value = !isExpanded.value;
};

// ⚠️ 不自动展开（保持收起），如需自动展开再改这里
watch(
  () => props.content,
  () => {}
);
</script>

<style scoped>
.think-block {
  margin-bottom: 0.75rem;
  border: 1px solid #e5e7eb;
  border-radius: 0.5rem;
  overflow: hidden;
  background-color: rgba(249, 250, 251, 0.5);
}

.think-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  cursor: pointer;
  transition: background-color 0.2s;
  border-bottom: 1px solid rgba(229, 231, 235, 0.5);
}
.think-header:hover {
  background-color: rgba(243, 244, 246, 0.5);
}
.think-header.expanded {
  background-color: rgba(243, 244, 246, 0.3);
}

.think-icon {
  color: #6b7280;
}
.think-label {
  flex: 1;
  font-size: 0.875rem;
  font-weight: 500;
  color: #4b5563;
}
.think-toggle {
  color: #9ca3af;
}

.think-content {
  padding: 0.5rem 0.75rem;
  background-color: rgba(255, 255, 255, 0.5);
}
.think-text {
  font-size: 0.875rem;
  color: #374151;
  line-height: 1.625;
  white-space: pre-wrap;
}

/* “思考中”三个点动画 */
.dots {
  display: inline-flex;
  gap: 3px;
  margin-left: 2px;
  vertical-align: middle;
}
.dot {
  width: 0.375rem;
  height: 0.375rem;
  border-radius: 9999px;
  background: #6b7280;
  opacity: 0.6;
  animation: pulse 1.2s infinite ease-in-out;
}
.dot:nth-child(2) {
  animation-delay: 0.15s;
}
.dot:nth-child(3) {
  animation-delay: 0.3s;
}
@keyframes pulse {
  0%,
  80%,
  100% {
    transform: scale(0.6);
    opacity: 0.4;
  }
  40% {
    transform: scale(1);
    opacity: 1;
  }
}

/* 展开/收起动画 */
.think-content-enter-active,
.think-content-leave-active {
  transition: all 0.3s ease;
  overflow: hidden;
}
.think-content-enter-from,
.think-content-leave-to {
  max-height: 0;
  opacity: 0;
  padding-top: 0;
  padding-bottom: 0;
}
.think-content-enter-to,
.think-content-leave-from {
  max-height: 500px;
  opacity: 1;
}

/* 暗色主题 */
.dark .think-block {
  border-color: #374151;
  background-color: rgba(31, 41, 55, 0.5);
}
.dark .think-header {
  border-bottom-color: rgba(55, 65, 81, 0.5);
}
.dark .think-header:hover {
  background-color: rgba(55, 65, 81, 0.5);
}
.dark .think-header.expanded {
  background-color: rgba(55, 65, 81, 0.3);
}
.dark .think-icon {
  color: #9ca3af;
}
.dark .think-label {
  color: #d1d5db;
}
.dark .think-toggle {
  color: #6b7280;
}
.dark .think-content {
  background-color: rgba(31, 41, 55, 0.3);
}
.dark .think-text {
  color: #d1d5db;
}
</style>
