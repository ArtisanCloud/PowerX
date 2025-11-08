<script setup lang="ts">
import { use } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import { LineChart, PieChart, BarChart } from "echarts/charts";
import {
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
  DataZoomComponent,
} from "echarts/components";
import VChart from "vue-echarts";
import type { EChartsOption } from "echarts";

// 注册 ECharts 组件
use([
  CanvasRenderer,
  LineChart,
  PieChart,
  BarChart,
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
  DataZoomComponent,
]);

const { t } = useI18n();

definePageMeta({
  layout: "default",
});

useHead({
  title: t("dashboard.title"),
  meta: [{ name: "description", content: t("dashboard.description") }],
});

// 统计数据
const stats = ref([
  {
    title: t("dashboard.stats.totalUsers"),
    value: "12,345",
    change: "+12%",
    changeType: "positive",
    icon: "i-heroicons-users",
  },
  {
    title: t("dashboard.stats.todayVisits"),
    value: "2,847",
    change: "+5.2%",
    changeType: "positive",
    icon: "i-heroicons-eye",
  },
  {
    title: t("dashboard.stats.totalRevenue"),
    value: "¥89,432",
    change: "-2.1%",
    changeType: "negative",
    icon: "i-heroicons-currency-yen",
  },
  {
    title: t("dashboard.stats.activeUsers"),
    value: "8,921",
    change: "+8.7%",
    changeType: "positive",
    icon: "i-heroicons-chart-bar-square",
  },
]);

// Agent 相关统计数据
const agentStats = ref([
  {
    title: "已安装 Agent",
    value: "24",
    change: "+3",
    changeType: "positive",
    icon: "i-heroicons-cpu-chip",
    description: "本月新增",
  },
  {
    title: "活跃 Agent",
    value: "18",
    change: "+2",
    changeType: "positive",
    icon: "i-heroicons-bolt",
    description: "今日活跃",
  },
  {
    title: "Token 消耗",
    value: "2.4M",
    change: "+15.3%",
    changeType: "positive",
    icon: "i-heroicons-currency-dollar",
    description: "本月总计",
  },
  {
    title: "对话次数",
    value: "8,432",
    change: "+22.1%",
    changeType: "positive",
    icon: "i-heroicons-chat-bubble-left-right",
    description: "本月总计",
  },
]);

// 访问趋势图表配置
const visitTrendOption = computed<EChartsOption>(() => ({
  title: {
    text: "访问趋势",
    left: "center",
    textStyle: {
      fontSize: 16,
      fontWeight: "normal",
    },
  },
  tooltip: {
    trigger: "axis",
    axisPointer: {
      type: "cross",
    },
  },
  grid: {
    left: "3%",
    right: "4%",
    bottom: "3%",
    containLabel: true,
  },
  xAxis: {
    type: "category",
    data: [
      "1月",
      "2月",
      "3月",
      "4月",
      "5月",
      "6月",
      "7月",
      "8月",
      "9月",
      "10月",
      "11月",
      "12月",
    ],
  },
  yAxis: {
    type: "value",
  },
  series: [
    {
      name: "页面访问量",
      type: "line",
      smooth: true,
      data: [
        1200, 1320, 1010, 1340, 1890, 2300, 2100, 2400, 2200, 1800, 1600, 1900,
      ],
      itemStyle: {
        color: "#3b82f6",
      },
      areaStyle: {
        color: {
          type: "linear",
          x: 0,
          y: 0,
          x2: 0,
          y2: 1,
          colorStops: [
            {
              offset: 0,
              color: "rgba(59, 130, 246, 0.3)",
            },
            {
              offset: 1,
              color: "rgba(59, 130, 246, 0.05)",
            },
          ],
        },
      },
    },
    {
      name: "独立访客",
      type: "line",
      smooth: true,
      data: [
        800, 900, 700, 950, 1200, 1500, 1300, 1600, 1400, 1100, 1000, 1250,
      ],
      itemStyle: {
        color: "#10b981",
      },
    },
  ],
}));

// 用户分布饼图配置
const userDistributionOption = computed<EChartsOption>(() => ({
  title: {
    text: "用户分布",
    left: "center",
    textStyle: {
      fontSize: 16,
      fontWeight: "normal",
    },
  },
  tooltip: {
    trigger: "item",
    formatter: "{a} <br/>{b}: {c} ({d}%)",
  },
  legend: {
    bottom: "5%",
    left: "center",
  },
  series: [
    {
      name: "用户分布",
      type: "pie",
      radius: ["40%", "70%"],
      center: ["50%", "45%"],
      avoidLabelOverlap: false,
      itemStyle: {
        borderRadius: 10,
        borderColor: "#fff",
        borderWidth: 2,
      },
      label: {
        show: false,
        position: "center",
      },
      emphasis: {
        label: {
          show: true,
          fontSize: 20,
          fontWeight: "bold",
        },
      },
      labelLine: {
        show: false,
      },
      data: [
        { value: 4200, name: "新用户", itemStyle: { color: "#3b82f6" } },
        { value: 3100, name: "活跃用户", itemStyle: { color: "#10b981" } },
        { value: 2800, name: "回访用户", itemStyle: { color: "#f59e0b" } },
        { value: 2245, name: "沉睡用户", itemStyle: { color: "#ef4444" } },
      ],
    },
  ],
}));

// 设备类型饼图配置
const deviceTypeOption = computed<EChartsOption>(() => ({
  title: {
    text: "设备类型",
    left: "center",
    textStyle: {
      fontSize: 16,
      fontWeight: "normal",
    },
  },
  tooltip: {
    trigger: "item",
    formatter: "{a} <br/>{b}: {c} ({d}%)",
  },
  legend: {
    bottom: "5%",
    left: "center",
  },
  series: [
    {
      name: "设备类型",
      type: "pie",
      radius: "60%",
      center: ["50%", "45%"],
      data: [
        { value: 6500, name: "移动端", itemStyle: { color: "#8b5cf6" } },
        { value: 4200, name: "桌面端", itemStyle: { color: "#06b6d4" } },
        { value: 1800, name: "平板端", itemStyle: { color: "#84cc16" } },
      ],
      emphasis: {
        itemStyle: {
          shadowBlur: 10,
          shadowOffsetX: 0,
          shadowColor: "rgba(0, 0, 0, 0.5)",
        },
      },
    },
  ],
}));

// Token 消耗趋势图配置
const tokenTrendOption = computed<EChartsOption>(() => ({
  title: {
    text: "Token 消耗趋势",
    left: "center",
    textStyle: {
      fontSize: 16,
      fontWeight: "normal",
    },
  },
  tooltip: {
    trigger: "axis",
    axisPointer: {
      type: "cross",
    },
  },
  grid: {
    left: "3%",
    right: "4%",
    bottom: "3%",
    containLabel: true,
  },
  xAxis: {
    type: "category",
    data: [
      "1月",
      "2月",
      "3月",
      "4月",
      "5月",
      "6月",
      "7月",
      "8月",
      "9月",
      "10月",
      "11月",
      "12月",
    ],
  },
  yAxis: {
    type: "value",
    name: "Token (K)",
  },
  series: [
    {
      name: "Token 消耗",
      type: "bar",
      data: [120, 200, 150, 180, 220, 300, 280, 350, 320, 280, 240, 290],
      itemStyle: {
        color: {
          type: "linear",
          x: 0,
          y: 0,
          x2: 0,
          y2: 1,
          colorStops: [
            {
              offset: 0,
              color: "#8b5cf6",
            },
            {
              offset: 1,
              color: "#a78bfa",
            },
          ],
        },
      },
    },
  ],
}));

// Agent 使用状态饼图配置
const agentStatusOption = computed<EChartsOption>(() => ({
  title: {
    text: "Agent 使用状态",
    left: "center",
    textStyle: {
      fontSize: 16,
      fontWeight: "normal",
    },
  },
  tooltip: {
    trigger: "item",
    formatter: "{a} <br/>{b}: {c} ({d}%)",
  },
  legend: {
    bottom: "5%",
    left: "center",
  },
  series: [
    {
      name: "Agent 状态",
      type: "pie",
      radius: "60%",
      center: ["50%", "45%"],
      data: [
        { value: 18, name: "活跃中", itemStyle: { color: "#10b981" } },
        { value: 4, name: "空闲中", itemStyle: { color: "#f59e0b" } },
        { value: 2, name: "维护中", itemStyle: { color: "#ef4444" } },
      ],
      emphasis: {
        itemStyle: {
          shadowBlur: 10,
          shadowOffsetX: 0,
          shadowColor: "rgba(0, 0, 0, 0.5)",
        },
      },
    },
  ],
}));

// Agent 类型分布饼图配置
const agentTypeOption = computed<EChartsOption>(() => ({
  title: {
    text: "Agent 类型分布",
    left: "center",
    textStyle: {
      fontSize: 16,
      fontWeight: "normal",
    },
  },
  tooltip: {
    trigger: "item",
    formatter: "{a} <br/>{b}: {c} ({d}%)",
  },
  legend: {
    bottom: "5%",
    left: "center",
  },
  series: [
    {
      name: "Agent 类型",
      type: "pie",
      radius: ["40%", "70%"],
      center: ["50%", "45%"],
      avoidLabelOverlap: false,
      itemStyle: {
        borderRadius: 10,
        borderColor: "#fff",
        borderWidth: 2,
      },
      label: {
        show: false,
        position: "center",
      },
      emphasis: {
        label: {
          show: true,
          fontSize: 20,
          fontWeight: "bold",
        },
      },
      labelLine: {
        show: false,
      },
      data: [
        { value: 8, name: "客服助手", itemStyle: { color: "#3b82f6" } },
        { value: 6, name: "内容创作", itemStyle: { color: "#10b981" } },
        { value: 4, name: "数据分析", itemStyle: { color: "#f59e0b" } },
        { value: 3, name: "代码助手", itemStyle: { color: "#8b5cf6" } },
        { value: 3, name: "其他类型", itemStyle: { color: "#ef4444" } },
      ],
    },
  ],
}));

// 收入来源饼图配置
const revenueSourceOption = computed<EChartsOption>(() => ({
  title: {
    text: "收入来源",
    left: "center",
    textStyle: {
      fontSize: 16,
      fontWeight: "normal",
    },
  },
  tooltip: {
    trigger: "item",
    formatter: "{a} <br/>{b}: ¥{c} ({d}%)",
  },
  legend: {
    bottom: "5%",
    left: "center",
  },
  series: [
    {
      name: "收入来源",
      type: "pie",
      radius: ["30%", "70%"],
      center: ["50%", "45%"],
      roseType: "area",
      itemStyle: {
        borderRadius: 8,
      },
      data: [
        { value: 35000, name: "产品销售", itemStyle: { color: "#f43f5e" } },
        { value: 28000, name: "服务费用", itemStyle: { color: "#3b82f6" } },
        { value: 15000, name: "广告收入", itemStyle: { color: "#10b981" } },
        { value: 11432, name: "其他收入", itemStyle: { color: "#f59e0b" } },
      ],
    },
  ],
}));

// 最近活动
const recentActivities = ref([
  {
    id: 1,
    user: t("dashboard.activities.users.zhangsan"),
    action: t("dashboard.activities.actions.createArticle"),
    target: t("dashboard.activities.targets.vueBestPractices"),
    time: t("dashboard.activities.times.minutesAgo", { minutes: 2 }),
    type: "create",
  },
  {
    id: 2,
    user: t("dashboard.activities.users.lisi"),
    action: t("dashboard.activities.actions.updateProfile"),
    target: "",
    time: t("dashboard.activities.times.minutesAgo", { minutes: 5 }),
    type: "update",
  },
  {
    id: 3,
    user: t("dashboard.activities.users.wangwu"),
    action: t("dashboard.activities.actions.deleteComment"),
    target: t("dashboard.activities.targets.onTechShare"),
    time: t("dashboard.activities.times.minutesAgo", { minutes: 10 }),
    type: "delete",
  },
]);
</script>

<template>
  <div class="space-y-6 p-4">
    <!-- 统计卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
      <UCard v-for="stat in stats" :key="stat.title" class="p-6">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-600">{{ stat.title }}</p>
            <p class="text-2xl font-bold text-gray-900 mt-2">
              {{ stat.value }}
            </p>
            <div class="flex items-center mt-2">
              <span
                class="text-sm font-medium"
                :class="{
                  'text-green-600': stat.changeType === 'positive',
                  'text-red-600': stat.changeType === 'negative',
                }"
              >
                {{ stat.change }}
              </span>
              <span class="text-sm text-gray-500 ml-1">{{
                $t("dashboard.stats.vsLastMonth")
              }}</span>
            </div>
          </div>
          <div class="p-3 bg-blue-50 rounded-lg">
            <UIcon
              class="w-6 h-6 text-blue-600 inline-block"
              :name="stat.icon"
            />
          </div>
        </div>
      </UCard>
    </div>

    <!-- 访问趋势图表 -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <!-- 访问趋势线图 -->
      <div class="lg:col-span-2">
        <UCard>
          <template #header>
            <div class="flex items-center justify-between">
              <h3 class="text-lg font-semibold text-gray-900">访问趋势分析</h3>
              <UButton variant="ghost" size="sm"> 查看详情 </UButton>
            </div>
          </template>

          <div class="h-80">
            <VChart :option="visitTrendOption" class="w-full h-full" />
          </div>
        </UCard>
      </div>

      <!-- 最近活动 -->
      <div>
        <UCard>
          <template #header>
            <h3 class="text-lg font-semibold text-gray-900">
              {{ $t("dashboard.activities.title") }}
            </h3>
          </template>

          <div class="space-y-4">
            <div
              v-for="activity in recentActivities"
              :key="activity.id"
              class="flex items-start space-x-3"
            >
              <div
                class="w-8 h-8 rounded-full flex items-center justify-center text-xs font-medium text-white"
                :class="{
                  'bg-green-500': activity.type === 'create',
                  'bg-blue-500': activity.type === 'update',
                  'bg-red-500': activity.type === 'delete',
                }"
              >
                {{ activity.user.charAt(0) }}
              </div>
              <div class="flex-1 min-w-0">
                <p class="text-sm text-gray-900">
                  <span class="font-medium">{{ activity.user }}</span>
                  {{ activity.action }}
                  <span v-if="activity.target" class="font-medium">{{
                    activity.target
                  }}</span>
                </p>
                <p class="text-xs text-gray-500 mt-1">{{ activity.time }}</p>
              </div>
            </div>
          </div>

          <template #footer>
            <UButton variant="ghost" block>
              {{ $t("dashboard.activities.viewAll") }}
            </UButton>
          </template>
        </UCard>
      </div>
    </div>

    <!-- Agent 统计卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
      <UCard v-for="stat in agentStats" :key="stat.title" class="p-6">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-600">{{ stat.title }}</p>
            <p class="text-2xl font-bold text-gray-900 mt-2">
              {{ stat.value }}
            </p>
            <div class="flex items-center mt-2">
              <span
                class="text-sm font-medium"
                :class="{
                  'text-green-600': stat.changeType === 'positive',
                  'text-red-600': stat.changeType === 'negative',
                }"
              >
                {{ stat.change }}
              </span>
              <span class="text-sm text-gray-500 ml-1">{{
                stat.description
              }}</span>
            </div>
          </div>
          <div class="p-3 bg-purple-50 rounded-lg">
            <UIcon
              class="w-6 h-6 text-purple-600 inline-block"
              :name="stat.icon"
            />
          </div>
        </div>
      </UCard>
    </div>

    <!-- Token 消耗趋势图 -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <!-- Token 消耗柱状图 -->
      <div class="lg:col-span-2">
        <UCard>
          <template #header>
            <div class="flex items-center justify-between">
              <h3 class="text-lg font-semibold text-gray-900">
                Token 消耗分析
              </h3>
              <UButton variant="ghost" size="sm"> 查看详情 </UButton>
            </div>
          </template>

          <div class="h-80">
            <VChart :option="tokenTrendOption" class="w-full h-full" />
          </div>
        </UCard>
      </div>

      <!-- Agent 使用状态饼图 -->
      <div>
        <UCard>
          <div class="h-80">
            <VChart :option="agentStatusOption" class="w-full h-full" />
          </div>
        </UCard>
      </div>
    </div>

    <!-- Agent 和业务数据饼图区域 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
      <!-- Agent 类型分布饼图 -->
      <UCard>
        <div class="h-80">
          <VChart :option="agentTypeOption" class="w-full h-full" />
        </div>
      </UCard>

      <!-- 用户分布饼图 -->
      <UCard>
        <div class="h-80">
          <VChart :option="userDistributionOption" class="w-full h-full" />
        </div>
      </UCard>

      <!-- 设备类型饼图 -->
      <UCard>
        <div class="h-80">
          <VChart :option="deviceTypeOption" class="w-full h-full" />
        </div>
      </UCard>

      <!-- 收入来源饼图 -->
      <UCard>
        <div class="h-80">
          <VChart :option="revenueSourceOption" class="w-full h-full" />
        </div>
      </UCard>
    </div>
  </div>
</template>
