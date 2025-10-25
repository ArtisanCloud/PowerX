<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { clearError } from '#app'
import type { NuxtError } from '#app'

// 接收 Nuxt 传入的错误对象
const props = defineProps<{ error: NuxtError | null }>()

// 全局主题状态（保留你的写法）
const theme = useState<'auto' | 'light' | 'dark'>('theme', () => 'auto')

// i18n
const { t } = useI18n()

// 复制状态
const copySuccess = ref(false)

// 标题（只定义一次）
const errorTitle = computed(() => {
  if (props.error?.statusCode === 404) {
    return t('error.notFound', '页面未找到')
  }
  return props.error?.statusMessage || t('error.occurred', '发生错误')
})

// 错误信息文本
const errorText = computed(() => {
  if (!props.error) return ''
  return JSON.stringify(props.error, null, 2)
})

// 行为：返回上一页 / 返回首页
const router = useRouter()

const handleGoBack = () => {
  // 如果没有历史记录就回首页，避免无效 back
  if (history.length > 1) {
    router.back()
  } else {
    clearError({ redirect: '/' })
  }
}

const handleGoHome = () => {
  // 清理错误并重定向到首页
  clearError({ redirect: '/' })
}

// 复制错误信息
const copyErrorInfo = async () => {
  try {
    await navigator.clipboard.writeText(errorText.value)
    copySuccess.value = true
    setTimeout(() => {
      copySuccess.value = false
    }, 2000)
  } catch (err) {
    console.error('复制失败:', err)
    // 降级方案：选中文本
    const textArea = document.createElement('textarea')
    textArea.value = errorText.value
    document.body.appendChild(textArea)
    textArea.select()
    document.execCommand('copy')
    document.body.removeChild(textArea)
    copySuccess.value = true
    setTimeout(() => {
      copySuccess.value = false
    }, 2000)
  }
}
</script>

<template>
  <div class="min-h-screen flex flex-col items-center justify-center px-6 py-12 text-center">
    <h1 class="text-2xl md:text-3xl font-semibold mb-4">
      {{ errorTitle }}
    </h1>

    <p class="text-gray-500 dark:text-gray-400 mb-4">
      {{ t('error.tip', '请尝试返回或回到首页。') }}
    </p>

    <!-- 显示详细错误信息 -->
    <div v-if="props.error" class="mb-8 max-w-2xl mx-auto">
      <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-left overflow-auto max-h-60 relative">
        <div class="flex items-center justify-between mb-2">
          <p class="font-medium text-red-700 dark:text-red-400">错误详情：</p>
          <button
            @click="copyErrorInfo"
            type="button"
            class="inline-flex items-center px-3 py-1 text-xs font-medium rounded-md transition-colors
                   bg-red-100 dark:bg-red-800/50 text-red-700 dark:text-red-300
                   hover:bg-red-200 dark:hover:bg-red-800/70
                   focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-1"
            :class="{ 'bg-green-100 dark:bg-green-800/50 text-green-700 dark:text-green-300': copySuccess }"
          >
            <svg v-if="!copySuccess" class="w-3 h-3 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path>
            </svg>
            <svg v-else class="w-3 h-3 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
            </svg>
            {{ copySuccess ? '已复制' : '复制' }}
          </button>
        </div>
        <pre class="text-sm text-red-600 dark:text-red-300 whitespace-pre-wrap break-words">{{ errorText }}</pre>
      </div>
    </div>

    <div class="flex justify-center space-x-4">
      <button
        @click="handleGoBack"
        type="button"
        class="not-prose appearance-none
                px-6 py-2 rounded-lg transition-colors
                !bg-gray-200 dark:!bg-gray-700
                !text-gray-700 dark:!text-gray-200
                hover:!bg-gray-300 dark:hover:!bg-gray-600
                border border-gray-300/70 dark:border-transparent
                focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
        >
        {{ t('error.goBack', '返回上一页') }}
        </button>
      <button
        @click="handleGoHome"
        type="button"
        class="not-prose appearance-none
                px-6 py-2 rounded-lg transition-colors
                !bg-blue-600 !text-white
                hover:!bg-blue-700
                focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
        >
        {{ t('error.goHome', '返回首页') }}
        </button>
    </div>
  </div>
</template>
