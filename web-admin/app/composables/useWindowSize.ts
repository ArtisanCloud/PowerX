import { ref, onMounted, onUnmounted } from 'vue'

/**
 * 响应式窗口尺寸钩子函数
 * 提供窗口的宽度和高度的响应式引用
 */
export function useWindowSize() {
  // 初始化为默认值，避免服务器端渲染时的错误
  const width = ref(0)
  const height = ref(0)

  function handleResize() {
    width.value = window.innerWidth
    height.value = window.innerHeight
  }

  // 只在客户端执行
  if (typeof window !== 'undefined') {
    // 初始化尺寸
    width.value = window.innerWidth
    height.value = window.innerHeight
    
    onMounted(() => {
      window.addEventListener('resize', handleResize)
    })

    onUnmounted(() => {
      window.removeEventListener('resize', handleResize)
    })
  }

  return { width, height }
}
