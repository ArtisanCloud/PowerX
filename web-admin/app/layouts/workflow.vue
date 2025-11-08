<script setup lang="ts">
import { ref, computed } from 'vue'

// 主题支持
const colorMode = useColorMode()
const isDark = computed(() => colorMode.value === 'dark')

// 工具栏状态
const showToolbar = ref(true)
const showMinimap = ref(true)
const showProperties = ref(true)

// 切换面板显示状态
const toggleToolbar = () => {
  showToolbar.value = !showToolbar.value
}

const toggleMinimap = () => {
  showMinimap.value = !showMinimap.value
}

const toggleProperties = () => {
  showProperties.value = !showProperties.value
}

// 全屏模式
const isFullscreen = ref(false)
const toggleFullscreen = () => {
  isFullscreen.value = !isFullscreen.value
  if (isFullscreen.value) {
    document.documentElement.requestFullscreen?.()
  } else {
    document.exitFullscreen?.()
  }
}
</script>

<template>
  <div class="workflow-layout h-screen flex flex-col overflow-hidden" :class="isDark ? 'bg-gray-900' : 'bg-gray-50'">
    <!-- 顶部工具栏 -->
    <div 
      v-if="showToolbar"
      class="workflow-toolbar px-4 py-2 flex items-center justify-between"
      :class="isDark ? 'bg-gray-800 border-b border-gray-700' : 'bg-white border-b border-gray-200'"
    >
      <!-- 左侧工具组 -->
      <div class="flex items-center space-x-4">
        <!-- 返回按钮 -->
        <NuxtLink 
          to="/workflow" 
          class="flex items-center transition-colors"
          :class="isDark ? 'text-gray-300 hover:text-white' : 'text-gray-600 hover:text-gray-900'"
        >
          <Icon name="i-heroicons-arrow-left" class="mr-2" />
          返回工作流列表
        </NuxtLink>
        
        <div class="h-6 w-px" :class="isDark ? 'bg-gray-600' : 'bg-gray-300'"></div>
        
        <!-- 工作流操作 -->
        <div class="flex items-center space-x-2">
          <UButton size="sm" color="primary" icon="i-heroicons-document-arrow-down">
            保存
          </UButton>
          <UButton size="sm" color="neutral" variant="ghost" icon="i-heroicons-play">
            运行
          </UButton>
          <UButton size="sm" color="neutral" variant="ghost" icon="i-heroicons-stop">
            停止
          </UButton>
        </div>
      </div>

      <!-- 中间标题 -->
      <div class="flex-1 text-center">
        <h1 class="font-medium" :class="isDark ? 'text-white' : 'text-gray-900'">工作流编辑器</h1>
      </div>

      <!-- 右侧工具组 -->
      <div class="flex items-center space-x-2">
        <!-- 视图控制 -->
        <UButton 
          size="sm" 
          color="neutral" 
          variant="ghost" 
          :icon="showMinimap ? 'i-heroicons-map' : 'i-heroicons-map'"
          @click="toggleMinimap"
        >
          小地图
        </UButton>
        
        <UButton 
          size="sm" 
          color="neutral" 
          variant="ghost" 
          :icon="showProperties ? 'i-heroicons-cog-6-tooth' : 'i-heroicons-cog-6-tooth'"
          @click="toggleProperties"
        >
          属性面板
        </UButton>
        
        <div class="h-6 w-px bg-gray-600"></div>
        
        <!-- 全屏切换 -->
        <UButton 
          size="sm" 
          color="neutral" 
          variant="ghost" 
          :icon="isFullscreen ? 'i-heroicons-arrows-pointing-in' : 'i-heroicons-arrows-pointing-out'"
          @click="toggleFullscreen"
        >
          {{ isFullscreen ? '退出全屏' : '全屏' }}
        </UButton>
        
        <!-- 隐藏工具栏 -->
        <UButton 
          size="sm" 
          color="neutral" 
          variant="ghost" 
          icon="i-heroicons-chevron-up"
          @click="toggleToolbar"
        >
          隐藏
        </UButton>
      </div>
    </div>

    <!-- 显示工具栏按钮（当工具栏隐藏时） -->
    <div 
      v-if="!showToolbar"
      class="absolute top-2 left-1/2 transform -translate-x-1/2 z-50"
    >
      <UButton 
        size="sm" 
        color="neutral" 
        variant="ghost" 
        icon="i-heroicons-chevron-down"
        @click="toggleToolbar"
      >
        显示工具栏
      </UButton>
    </div>

    <!-- 主要工作区域 -->
    <div class="flex-1 flex overflow-hidden">
      <!-- 主编辑区域 -->
      <div class="flex-1 relative overflow-hidden">
        <NuxtPage />
      </div>

      <!-- 右侧属性面板 -->
      <div 
        v-if="showProperties"
        class="w-80 flex flex-col"
        :class="isDark ? 'bg-gray-800 border-l border-gray-700' : 'bg-white border-l border-gray-200'"
      >
        <div class="p-4" :class="isDark ? 'border-b border-gray-700' : 'border-b border-gray-200'">
          <div class="flex items-center justify-between">
            <h3 class="font-medium" :class="isDark ? 'text-white' : 'text-gray-900'">属性面板</h3>
            <UButton 
              size="xs" 
              color="neutral" 
              variant="ghost" 
              icon="i-heroicons-x-mark"
              @click="toggleProperties"
            />
          </div>
        </div>
        
        <div class="flex-1 overflow-auto p-4">
          <div class="text-sm" :class="isDark ? 'text-gray-400' : 'text-gray-500'">
            选择节点以查看属性
          </div>
        </div>
      </div>
    </div>

    <!-- 底部小地图 -->
    <div 
      v-if="showMinimap"
      class="absolute bottom-4 right-4 w-64 h-40 rounded-lg overflow-hidden"
      :class="isDark ? 'bg-gray-800 border border-gray-700' : 'bg-white border border-gray-200'"
    >
      <div class="p-2" :class="isDark ? 'bg-gray-700 border-b border-gray-600' : 'bg-gray-50 border-b border-gray-200'">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium" :class="isDark ? 'text-white' : 'text-gray-900'">小地图</span>
          <UButton 
            size="xs" 
            color="neutral" 
            variant="ghost" 
            icon="i-heroicons-x-mark"
            @click="toggleMinimap"
          />
        </div>
      </div>
      <div class="flex-1 relative" :class="isDark ? 'bg-gray-900' : 'bg-gray-100'">
        <div class="absolute inset-0 flex items-center justify-center text-xs" :class="isDark ? 'text-gray-500' : 'text-gray-400'">
          工作流概览
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.workflow-layout {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

.workflow-toolbar {
  min-height: 48px;
}

/* 确保全屏模式下的样式 */
:fullscreen .workflow-layout {
  height: 100vh;
}
</style>