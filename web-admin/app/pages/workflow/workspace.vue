<template>
  <div class="workflow-workspace h-full" :class="isDark ? 'bg-gray-900' : 'bg-gray-50'">
    <WorkflowEditor />
  </div>
</template>

<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { useColorMode, useI18n } from '#imports'
import { useRoute } from 'vue-router'
import WorkflowEditor from '~/components/workflow/WorkflowEditor.vue'

// 主题支持
const colorMode = useColorMode()
const isDark = computed(() => colorMode.value === 'dark')

// 使用工作流布局
definePageMeta({
  layout: 'workflow'
})

// 路由和工作流管理
const route = useRoute()
const { t } = useI18n()

onMounted(async () => {
  const workflowId = route.query.id as string
  
  if (!workflowId) {
    await navigateTo('/workflow')
  }
})

// 设置页面标题
useHead({
  title: t('workflow.editor.pageTitle')
})
</script>

<style scoped>
.workflow-workspace {
  /* 背景色现在通过模板中的类名动态设置 */
}
</style>
