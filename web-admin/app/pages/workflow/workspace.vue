<template>
  <div class="workflow-workspace h-full" :class="isDark ? 'bg-gray-900' : 'bg-gray-50'">
    <WorkflowEditor />
  </div>
</template>

<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { useColorMode } from '#imports'
import { useRoute } from 'vue-router'
import { useWorkflowManager } from '~/composables/workflow/useWorkflowManager'
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
const { loadWorkflow, currentWorkflow } = useWorkflowManager()

// 页面挂载时加载工作流
onMounted(async () => {
  const workflowId = route.query.id as string
  
  if (workflowId) {
    try {
      await loadWorkflow(workflowId)
      console.log('工作流加载成功:', currentWorkflow.value)
    } catch (error) {
      console.error('加载工作流失败:', error)
      // 可以添加错误提示，但不立即重定向
      // 显示一个错误提示而不是重定向
    }
  } else {
    // 创建一个演示工作流，展示前端功能
    console.log('创建演示工作流')
    currentWorkflow.value = {
      id: 'demo-workflow',
      name: '演示工作流',
      description: '展示前端工作流编辑器功能',
      nodes: [],
      edges: [],
      version: '1.0.0',
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString()
    }
  }
})

// 设置页面标题
useHead({
  title: '工作流编辑器 - PowerX'
})
</script>

<style scoped>
.workflow-workspace {
  /* 背景色现在通过模板中的类名动态设置 */
}
</style>
