<script setup lang="ts">
// 页面元数据
definePageMeta({
  title: "布局演示",
  description: "展示新的现代化布局设计",
  layout: "default",
});

// 示例数据
const cards = ref([
  {
    id: 1,
    title: "现代化设计",
    description: "采用毛玻璃效果和渐变背景，提升视觉层次感",
    icon: "i-heroicons-sparkles",
    color: "blue",
  },
  {
    id: 2,
    title: "响应式布局",
    description: "完美适配各种屏幕尺寸，提供一致的用户体验",
    icon: "i-heroicons-device-phone-mobile",
    color: "green",
  },
  {
    id: 3,
    title: "阴影层次",
    description: "通过精心设计的阴影系统，创造深度和层次感",
    icon: "i-heroicons-cube-transparent",
    color: "purple",
  },
  {
    id: 4,
    title: "流畅动画",
    description: "平滑的过渡动画，提升用户交互体验",
    icon: "i-heroicons-bolt",
    color: "yellow",
  },
  {
    id: 5,
    title: "毛玻璃效果",
    description: "backdrop-blur 创造现代感十足的透明效果",
    icon: "i-heroicons-eye",
    color: "indigo",
  },
  {
    id: 6,
    title: "渐变背景",
    description: "精心调配的渐变色彩，营造温馨的视觉氛围",
    icon: "i-heroicons-paint-brush",
    color: "pink",
  },
]);

const stats = ref([
  { label: "总用户数", value: "12,345", change: "+12%", trend: "up" },
  { label: "活跃用户", value: "8,901", change: "+8%", trend: "up" },
  { label: "新增订单", value: "234", change: "-3%", trend: "down" },
  { label: "收入", value: "¥45,678", change: "+15%", trend: "up" },
]);
</script>

<template>
  <div class="space-y-8">
    <!-- 页面标题 -->
    <div class="text-center">
      <h1 class="text-3xl font-bold text-gray-900 dark:text-white mb-4">
        🎨 现代化布局演示
      </h1>
      <p class="text-lg text-gray-600 dark:text-gray-300 max-w-2xl mx-auto">
        体验全新的视觉设计：毛玻璃效果、渐变背景、精致阴影，完美支持深色模式的现代化界面
      </p>
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
      <div
        v-for="stat in stats"
        :key="stat.label"
        class="bg-white/80 backdrop-blur-sm rounded-xl shadow-lg border border-gray-200/60 p-6 hover:shadow-xl transition-all duration-300 hover:-translate-y-1"
      >
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-600">{{ stat.label }}</p>
            <p class="text-2xl font-bold text-gray-900 mt-1">
              {{ stat.value }}
            </p>
          </div>
          <div
            class="flex items-center text-sm"
            :class="stat.trend === 'up' ? 'text-green-600' : 'text-red-600'"
          >
            <UIcon
              :name="
                stat.trend === 'up'
                  ? 'i-heroicons-arrow-trending-up'
                  : 'i-heroicons-arrow-trending-down'
              "
              class="w-4 h-4 mr-1"
            />
            {{ stat.change }}
          </div>
        </div>
      </div>
    </div>

    <!-- 功能特性卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      <div
        v-for="card in cards"
        :key="card.id"
        class="group bg-white/80 backdrop-blur-sm rounded-xl shadow-lg border border-gray-200/60 p-6 hover:shadow-xl transition-all duration-300 hover:-translate-y-2 cursor-pointer"
      >
        <div class="flex items-start space-x-4">
          <div
            class="flex-shrink-0 w-12 h-12 rounded-lg flex items-center justify-center transition-transform duration-300 group-hover:scale-110"
            :class="`bg-${card.color}-100`"
          >
            <UIcon
              :name="card.icon"
              :class="`w-6 h-6 text-${card.color}-600`"
            />
          </div>
          <div class="flex-1">
            <h3
              class="text-lg font-semibold text-gray-900 mb-2 group-hover:text-blue-600 transition-colors"
            >
              {{ card.title }}
            </h3>
            <p class="text-gray-600 text-sm leading-relaxed">
              {{ card.description }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- 布局特性展示 -->
    <div
      class="bg-white/80 backdrop-blur-sm rounded-xl shadow-lg border border-gray-200/60 p-8"
    >
      <h2 class="text-2xl font-bold text-gray-900 mb-6 text-center">
        🚀 布局优化特性
      </h2>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-8">
        <!-- 左侧：特性列表 -->
        <div class="space-y-4">
          <div class="flex items-start space-x-3">
            <div
              class="w-6 h-6 bg-green-100 rounded-full flex items-center justify-center mt-0.5"
            >
              <UIcon name="i-heroicons-check" class="w-4 h-4 text-green-600" />
            </div>
            <div>
              <h4 class="font-semibold text-gray-900">
                毛玻璃效果 (Backdrop Blur)
              </h4>
              <p class="text-sm text-gray-600">
                所有主要组件都采用了 backdrop-blur-sm 效果
              </p>
            </div>
          </div>

          <div class="flex items-start space-x-3">
            <div
              class="w-6 h-6 bg-green-100 rounded-full flex items-center justify-center mt-0.5"
            >
              <UIcon name="i-heroicons-check" class="w-4 h-4 text-green-600" />
            </div>
            <div>
              <h4 class="font-semibold text-gray-900">渐变背景</h4>
              <p class="text-sm text-gray-600">
                主背景使用 gradient-to-br 创造深度感
              </p>
            </div>
          </div>

          <div class="flex items-start space-x-3">
            <div
              class="w-6 h-6 bg-green-100 rounded-full flex items-center justify-center mt-0.5"
            >
              <UIcon name="i-heroicons-check" class="w-4 h-4 text-green-600" />
            </div>
            <div>
              <h4 class="font-semibold text-gray-900">精致阴影系统</h4>
              <p class="text-sm text-gray-600">
                从 shadow-sm 到 shadow-xl 的层次化阴影
              </p>
            </div>
          </div>

          <div class="flex items-start space-x-3">
            <div
              class="w-6 h-6 bg-green-100 rounded-full flex items-center justify-center mt-0.5"
            >
              <UIcon name="i-heroicons-check" class="w-4 h-4 text-green-600" />
            </div>
            <div>
              <h4 class="font-semibold text-gray-900">透明度优化</h4>
              <p class="text-sm text-gray-600">
                使用 /95、/80、/60 等透明度创造层次
              </p>
            </div>
          </div>

          <div class="flex items-start space-x-3">
            <div
              class="w-6 h-6 bg-green-100 rounded-full flex items-center justify-center mt-0.5"
            >
              <UIcon name="i-heroicons-check" class="w-4 h-4 text-green-600" />
            </div>
            <div>
              <h4 class="font-semibold text-gray-900 dark:text-white">
                深色模式支持
              </h4>
              <p class="text-sm text-gray-600 dark:text-gray-300">
                完美兼容深色模式，所有组件都有对应的深色样式
              </p>
            </div>
          </div>
        </div>

        <!-- 右侧：视觉示例 -->
        <div class="space-y-4">
          <div
            class="bg-gradient-to-r from-blue-50 to-purple-50 rounded-lg p-4 border border-blue-200/60"
          >
            <h4 class="font-semibold text-blue-900 mb-2">Header 区域</h4>
            <p class="text-sm text-blue-700">
              毛玻璃效果 + 粘性定位 + 精致阴影
            </p>
          </div>

          <div
            class="bg-gradient-to-r from-gray-50 to-blue-50 rounded-lg p-4 border border-gray-200/60"
          >
            <h4 class="font-semibold text-gray-900 mb-2">Sidebar 区域</h4>
            <p class="text-sm text-gray-700">
              渐变背景 + Logo 区域美化 + 用户信息区域
            </p>
          </div>

          <div
            class="bg-gradient-to-r from-green-50 to-blue-50 rounded-lg p-4 border border-green-200/60"
          >
            <h4 class="font-semibold text-green-900 mb-2">Content 区域</h4>
            <p class="text-sm text-green-700">圆角优化 + 内容区域透明效果</p>
          </div>

          <div
            class="bg-gradient-to-r from-purple-50 to-pink-50 rounded-lg p-4 border border-purple-200/60"
          >
            <h4 class="font-semibold text-purple-900 mb-2">Footer 区域</h4>
            <p class="text-sm text-purple-700">底部毛玻璃效果 + 边框透明度</p>
          </div>
        </div>
      </div>
    </div>

    <!-- 主题切换演示 -->
    <div
      class="bg-white/80 dark:bg-gray-900/80 backdrop-blur-sm rounded-xl shadow-lg border border-gray-200/60 dark:border-gray-700/60 p-8"
    >
      <h2
        class="text-2xl font-bold text-gray-900 dark:text-white mb-6 text-center"
      >
        🌙 深色模式兼容性
      </h2>

      <div class="text-center mb-6">
        <p class="text-gray-600 dark:text-gray-300 mb-4">
          所有布局组件都完美支持深色模式切换，保持一致的视觉体验
        </p>
        <div class="flex justify-center">
          <ThemeSwitcher />
        </div>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div class="space-y-3">
          <h4 class="font-semibold text-gray-900 dark:text-white">
            浅色模式特性
          </h4>
          <ul class="space-y-2 text-sm text-gray-600 dark:text-gray-300">
            <li class="flex items-center space-x-2">
              <UIcon name="i-heroicons-sun" class="w-4 h-4 text-yellow-500" />
              <span>清爽的白色背景和渐变</span>
            </li>
            <li class="flex items-center space-x-2">
              <UIcon name="i-heroicons-eye" class="w-4 h-4 text-blue-500" />
              <span>毛玻璃效果增强层次感</span>
            </li>
            <li class="flex items-center space-x-2">
              <UIcon
                name="i-heroicons-sparkles"
                class="w-4 h-4 text-purple-500"
              />
              <span>精致的阴影和边框</span>
            </li>
          </ul>
        </div>

        <div class="space-y-3">
          <h4 class="font-semibold text-gray-900 dark:text-white">
            深色模式特性
          </h4>
          <ul class="space-y-2 text-sm text-gray-600 dark:text-gray-300">
            <li class="flex items-center space-x-2">
              <UIcon name="i-heroicons-moon" class="w-4 h-4 text-indigo-400" />
              <span>护眼的深色背景和渐变</span>
            </li>
            <li class="flex items-center space-x-2">
              <UIcon
                name="i-heroicons-shield-check"
                class="w-4 h-4 text-green-400"
              />
              <span>自动适配的边框和文字颜色</span>
            </li>
            <li class="flex items-center space-x-2">
              <UIcon
                name="i-heroicons-cpu-chip"
                class="w-4 h-4 text-cyan-400"
              />
              <span>保持一致的交互体验</span>
            </li>
          </ul>
        </div>
      </div>
    </div>

    <!-- 交互演示 -->
    <div
      class="bg-white/80 dark:bg-gray-900/80 backdrop-blur-sm rounded-xl shadow-lg border border-gray-200/60 dark:border-gray-700/60 p-8"
    >
      <h2
        class="text-2xl font-bold text-gray-900 dark:text-white mb-6 text-center"
      >
        ✨ 交互效果演示
      </h2>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
        <div class="text-center">
          <div
            class="w-16 h-16 bg-gradient-to-r from-blue-500 to-purple-500 rounded-full mx-auto mb-4 shadow-lg hover:shadow-xl transition-all duration-300 hover:scale-110 cursor-pointer"
          ></div>
          <h4 class="font-semibold text-gray-900">悬停缩放</h4>
          <p class="text-sm text-gray-600 mt-1">鼠标悬停时的缩放效果</p>
        </div>

        <div class="text-center">
          <div
            class="w-16 h-16 bg-gradient-to-r from-green-500 to-blue-500 rounded-full mx-auto mb-4 shadow-lg hover:shadow-xl hover:-translate-y-2 transition-all duration-300 cursor-pointer"
          ></div>
          <h4 class="font-semibold text-gray-900">上浮效果</h4>
          <p class="text-sm text-gray-600 mt-1">悬停时的上浮动画</p>
        </div>

        <div class="text-center">
          <div
            class="w-16 h-16 bg-gradient-to-r from-pink-500 to-red-500 rounded-full mx-auto mb-4 shadow-lg hover:shadow-xl hover:rotate-12 transition-all duration-300 cursor-pointer"
          ></div>
          <h4 class="font-semibold text-gray-900">旋转效果</h4>
          <p class="text-sm text-gray-600 mt-1">悬停时的旋转动画</p>
        </div>
      </div>
    </div>
  </div>
</template>
