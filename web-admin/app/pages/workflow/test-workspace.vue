<template>
  <div class="workflow-workspace w-full h-full flex flex-col">
    <!-- 顶部切换（扁平结构，避免多层嵌套） -->
    <div
      class="flex items-center gap-2 p-2 border-b"
      style="
        border-color: var(--border-color);
        background-color: var(--bg-secondary);
      "
    >
      <UButton
        icon="i-heroicons-pencil-square"
        variant="ghost"
        size="sm"
        class="whitespace-nowrap gap-2"
        :color="activeTab === 'editor' ? 'primary' : 'neutral'"
        @click="activeTab = 'editor'"
      >
        基础编辑器
      </UButton>
      <UButton
        icon="i-heroicons-arrows-up-down"
        variant="ghost"
        size="sm"
        class="whitespace-nowrap gap-2"
        :color="activeTab === 'dnd' ? 'primary' : 'neutral'"
        @click="activeTab = 'dnd'"
      >
        拖拽示例
      </UButton>
    </div>

    <!-- 内容区，仅渲染一个组件，充满剩余空间 -->
    <div class="flex-1 min-h-0 w-full">
      <TestWorkflowEditor v-if="activeTab === 'editor'" />
      <TestDnDWorkflow v-else />
    </div>
  </div>
</template>

<script setup lang="ts">
import TestWorkflowEditor from "~/components/workflow/test/base/index.vue";
import TestDnDWorkflow from "~/components/workflow/test/drag-drop/index.vue";

const activeTab = ref<"editor" | "dnd">("editor");

// 禁用布局，避免样式冲突
definePageMeta({
  // layout: false
  layout: "workflow",
});

// 设置页面标题
useHead({
  title: "工作流编辑器测试 - PowerX",
});
</script>

<style scoped>
.workflow-workspace {
  width: 100%;
  height: 100%;
  background-color: var(--bg-primary);
  color: var(--text-primary);
}
</style>
