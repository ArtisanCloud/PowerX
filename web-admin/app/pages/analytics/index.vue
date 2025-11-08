<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">数据分析</h1>
      <p class="text-gray-600 dark:text-gray-400">查看网站访问和用户行为数据</p>
    </div>

    <!-- 时间范围选择 -->
    <div class="mb-6 flex justify-between items-center">
      <div class="flex gap-2">
        <UButton 
          v-for="period in timePeriods" 
          :key="period.value"
          :variant="selectedPeriod === period.value ? 'solid' : 'outline'"
          size="sm"
          @click="selectedPeriod = period.value"
        >
          {{ period.label }}
        </UButton>
      </div>
      
      <div class="flex gap-2">
        <UInput 
          v-model="dateRange.start"
          type="date"
          size="sm"
        />
        <span class="self-center text-gray-500">至</span>
        <UInput 
          v-model="dateRange.end"
          type="date"
          size="sm"
        />
      </div>
    </div>

    <!-- 核心指标 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
      <UCard v-for="metric in coreMetrics" :key="metric.title">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-600 dark:text-gray-400">{{ metric.title }}</p>
            <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{{ metric.value }}</p>
            <div class="flex items-center mt-2">
                            <UIcon 
                              :name="metric.trend === 'up' ? 'i-lucide-arrow-trending-up' : 'i-lucide-arrow-trending-down'"
                              class="w-4 h-4 metric.trend === 'up' ? 'text-green-500' : 'text-red-500' inline-block"
                              />
              <span 
                :class="metric.trend === 'up' ? 'text-green-600' : 'text-red-600'"
                class="text-sm font-medium ml-1"
              >
                {{ metric.change }}
              </span>
              <span class="text-sm text-gray-500 ml-1">vs 上期</span>
            </div>
          </div>
          <div class="p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
                        <span class="w-6 h-6 text-blue-600 inline-block">
                          <UIcon class="w-6 h-6 text-blue-600 inline-block" :name="metric.icon"  />
                        </span>
          </div>
        </div>
      </UCard>
    </div>

    <!-- 图表区域 -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-8">
      <!-- 访问趋势图 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">访问趋势</h3>
        </template>
        
        <div class="h-64 bg-gray-50 dark:bg-gray-800 rounded-lg flex items-center justify-center">
          <div class="text-center">
                        <span class="w-12 h-12 text-gray-400 mx-auto mb-2 inline-block">
                          <UIcon class="w-12 h-12 text-gray-400 mx-auto mb-2 inline-block" name="i-lucide-chart-bar"  />
                        </span>
            <p class="text-gray-500">访问趋势图表</p>
            <p class="text-sm text-gray-400">集成图表库后显示</p>
          </div>
        </div>
      </UCard>

      <!-- 用户来源 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">用户来源</h3>
        </template>
        
        <div class="space-y-4">
          <div v-for="source in trafficSources" :key="source.name" class="flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div 
                class="w-3 h-3 rounded-full"
                :style="{ backgroundColor: source.color }"
              ></div>
              <span class="font-medium">{{ source.name }}</span>
            </div>
            <div class="text-right">
              <div class="font-semibold">{{ source.visitors }}</div>
              <div class="text-sm text-gray-500">{{ source.percentage }}%</div>
            </div>
          </div>
        </div>
      </UCard>
    </div>

    <!-- 详细数据表格 -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- 热门页面 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">热门页面</h3>
        </template>
        
        <div class="space-y-3">
          <div v-for="page in popularPages" :key="page.path" class="flex items-center justify-between py-2">
            <div>
              <div class="font-medium">{{ page.title }}</div>
              <div class="text-sm text-gray-500">{{ page.path }}</div>
            </div>
            <div class="text-right">
              <div class="font-semibold">{{ page.views }}</div>
              <div class="text-sm text-gray-500">{{ page.uniqueViews }} 独立访问</div>
            </div>
          </div>
        </div>
      </UCard>

      <!-- 设备统计 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">设备统计</h3>
        </template>
        
        <div class="space-y-4">
          <div v-for="device in deviceStats" :key="device.type" class="flex items-center justify-between">
            <div class="flex items-center gap-3">
                            <span class="w-5 h-5 text-gray-600 inline-block">
                              <UIcon class="w-5 h-5 text-gray-600 inline-block" :name="device.icon"  />
                            </span>
              <span class="font-medium">{{ device.type }}</span>
            </div>
            <div class="text-right">
              <div class="font-semibold">{{ device.count }}</div>
              <div class="text-sm text-gray-500">{{ device.percentage }}%</div>
            </div>
          </div>
        </div>
      </UCard>
    </div>
  </div>
</template>

<script setup lang="ts">
// 页面元数据
definePageMeta({
  title: '数据分析',
  layout: 'default'
})

// 时间范围选择
const selectedPeriod = ref('7d')
const timePeriods = [
  { label: '今天', value: '1d' },
  { label: '7天', value: '7d' },
  { label: '30天', value: '30d' },
  { label: '90天', value: '90d' }
]

const dateRange = reactive({
  start: '',
  end: ''
})

// 核心指标数据
const coreMetrics = ref([
  {
    title: '总访问量',
    value: '24,567',
    change: '+12.5%',
    trend: 'up',
    icon: 'i-lucide-eye'
  },
  {
    title: '独立访客',
    value: '8,234',
    change: '+8.2%',
    trend: 'up',
    icon: 'i-lucide-users'
  },
  {
    title: '页面浏览量',
    value: '45,123',
    change: '+15.3%',
    trend: 'up',
    icon: 'i-lucide-document-text'
  },
  {
    title: '跳出率',
    value: '32.4%',
    change: '-2.1%',
    trend: 'up',
    icon: 'i-lucide-arrow-right-on-rectangle'
  }
])

// 流量来源数据
const trafficSources = ref([
  { name: '直接访问', visitors: '12,345', percentage: 45, color: '#3B82F6' },
  { name: '搜索引擎', visitors: '8,234', percentage: 30, color: '#10B981' },
  { name: '社交媒体', visitors: '4,567', percentage: 17, color: '#F59E0B' },
  { name: '其他', visitors: '2,123', percentage: 8, color: '#EF4444' }
])

// 热门页面数据
const popularPages = ref([
  { title: '首页', path: '/', views: '15,234', uniqueViews: '12,456' },
  { title: '产品页面', path: '/products', views: '8,567', uniqueViews: '6,789' },
  { title: '关于我们', path: '/about', views: '5,432', uniqueViews: '4,321' },
  { title: '联系我们', path: '/contact', views: '3,210', uniqueViews: '2,876' },
  { title: '博客', path: '/blog', views: '2,987', uniqueViews: '2,543' }
])

// 设备统计数据
const deviceStats = ref([
  { type: '桌面端', count: '18,456', percentage: 67, icon: 'i-lucide-computer-desktop' },
  { type: '移动端', count: '7,234', percentage: 26, icon: 'i-lucide-device-phone-mobile' },
  { type: '平板', count: '1,877', percentage: 7, icon: 'i-lucide-device-tablet' }
])
</script>