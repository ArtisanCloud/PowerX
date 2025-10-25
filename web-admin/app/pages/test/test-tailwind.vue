<template>
  <div class="min-h-screen bg-gradient-to-br from-blue-400 to-purple-600 p-8">
    <div class="max-w-4xl mx-auto">
      <h1 class="text-4xl font-bold text-white mb-8 text-center">Tailwind CSS 测试页面</h1>
      
      <!-- 基础样式测试 -->
      <div class="bg-white rounded-lg shadow-lg p-6 mb-6">
        <h2 class="text-2xl font-semibold text-gray-800 mb-4">基础样式测试</h2>
        <p class="text-gray-600 mb-4">如果您能看到这些样式，说明Tailwind CSS正在工作：</p>
        
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div class="bg-red-100 border border-red-300 rounded-md p-4">
            <h3 class="text-red-800 font-medium">红色卡片</h3>
            <p class="text-red-600 text-sm">这是红色主题的卡片</p>
          </div>
          
          <div class="bg-green-100 border border-green-300 rounded-md p-4">
            <h3 class="text-green-800 font-medium">绿色卡片</h3>
            <p class="text-green-600 text-sm">这是绿色主题的卡片</p>
          </div>
          
          <div class="bg-blue-100 border border-blue-300 rounded-md p-4">
            <h3 class="text-blue-800 font-medium">蓝色卡片</h3>
            <p class="text-blue-600 text-sm">这是蓝色主题的卡片</p>
          </div>
        </div>
      </div>
      
      <!-- 响应式测试 -->
      <div class="bg-white rounded-lg shadow-lg p-6 mb-6">
        <h2 class="text-2xl font-semibold text-gray-800 mb-4">响应式测试</h2>
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <div class="bg-purple-500 text-white p-4 rounded text-center">
            <div class="text-lg font-bold">1</div>
            <div class="text-sm">移动端：1列</div>
          </div>
          <div class="bg-purple-500 text-white p-4 rounded text-center">
            <div class="text-lg font-bold">2</div>
            <div class="text-sm">平板：2列</div>
          </div>
          <div class="bg-purple-500 text-white p-4 rounded text-center">
            <div class="text-lg font-bold">3</div>
            <div class="text-sm">桌面：4列</div>
          </div>
          <div class="bg-purple-500 text-white p-4 rounded text-center">
            <div class="text-lg font-bold">4</div>
            <div class="text-sm">调整窗口大小测试</div>
          </div>
        </div>
      </div>
      
      <!-- 动画和交互测试 -->
      <div class="bg-white rounded-lg shadow-lg p-6">
        <h2 class="text-2xl font-semibold text-gray-800 mb-4">动画和交互测试</h2>
        <div class="space-y-4">
          <button class="bg-blue-500 hover:bg-blue-600 text-white font-bold py-2 px-4 rounded transition-colors duration-200 transform hover:scale-105">
            悬停效果按钮
          </button>
          
          <div class="w-full bg-gray-200 rounded-full h-2.5">
            <div class="bg-gradient-to-r from-blue-500 to-purple-600 h-2.5 rounded-full animate-pulse" style="width: 75%"></div>
          </div>
          
          <div class="flex space-x-2">
            <div class="w-4 h-4 bg-red-500 rounded-full animate-bounce"></div>
            <div class="w-4 h-4 bg-green-500 rounded-full animate-bounce" style="animation-delay: 0.1s"></div>
            <div class="w-4 h-4 bg-blue-500 rounded-full animate-bounce" style="animation-delay: 0.2s"></div>
          </div>
        </div>
      </div>
      
      <!-- 返回链接 -->
      <div class="text-center mt-8">
        <NuxtLink to="/" class="inline-block bg-white text-purple-600 font-semibold py-2 px-6 rounded-lg shadow hover:shadow-lg transition-shadow duration-200">
          返回首页
        </NuxtLink>
      </div>
    </div>
  </div>
</template>

<script setup>
useHead({
  title: 'Tailwind CSS 测试页面',
  meta: [
    { name: 'description', content: '测试Tailwind CSS是否正常工作' }
  ]
})
</script>