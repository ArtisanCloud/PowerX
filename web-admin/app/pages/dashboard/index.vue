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
    title: "已安装插件",
    value: "8",
    change: "6 active",
    changeType: "positive",
    icon: "i-heroicons-puzzle-piece",
  },
  {
    title: "Gateway 调用",
    value: "18.4K",
    change: "99.3%",
    changeType: "positive",
    icon: "i-heroicons-bolt",
  },
  {
    title: "调度任务",
    value: "126",
    change: "24h",
    changeType: "positive",
    icon: "i-heroicons-clock",
  },
  {
    title: "AI / Knowledge",
    value: "42",
    change: "ready",
    changeType: "positive",
    icon: "i-heroicons-sparkles",
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

const platformModules = ref([
  {
    name: "插件运行时",
    desc: "安装、启用、健康检查、代理路由与动态页面",
    status: "Ready",
    icon: "i-heroicons-cube-transparent",
    tone: "blue",
  },
  {
    name: "Integration Gateway",
    desc: "STS / API Key、Capability Registry、调用 Trace",
    status: "Ready",
    icon: "i-heroicons-arrows-right-left",
    tone: "green",
  },
  {
    name: "Runtime Scheduler",
    desc: "插件业务 once / interval / cron job 统一托管",
    status: "Ready",
    icon: "i-heroicons-calendar-days",
    tone: "emerald",
  },
  {
    name: "AI Engine",
    desc: "模型 Provider、连接测试、LLM 调用入口",
    status: "Beta",
    icon: "i-heroicons-cpu-chip",
    tone: "violet",
  },
]);

const pluginPortfolio = ref([
  { name: "SCRM 插件", status: "开源仓库版本", desc: "客户、会话、触点与跟进协同" },
  { name: "电商插件", status: "开源仓库版本", desc: "商品、订单、交易与履约基础能力" },
  { name: "营销工具插件", status: "商用版本", desc: "营销自动化、活动编排与触达计划" },
]);

const runtimeEvents = ref([
  {
    title: "Runtime Scheduler job triggered",
    desc: "framework_lab_scheduler_probe -> system notification",
    time: "2 分钟前",
    tone: "green",
  },
  {
    title: "Gateway capability invoked",
    desc: "com.powerx.plugins.ai-craft / ai-engine.test",
    time: "8 分钟前",
    tone: "blue",
  },
  {
    title: "Plugin healthcheck passed",
    desc: "com.powerx.plugins.base active",
    time: "15 分钟前",
    tone: "emerald",
  },
]);

// 访问趋势图表配置
const visitTrendOption = computed<EChartsOption>(() => ({
  title: {
    text: "底座能力调用趋势",
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
        4200, 5100, 4600, 6200, 8100, 9600, 10400, 12800, 11700, 13500, 14200, 15600,
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
      name: "Gateway 调用",
      type: "line",
      smooth: true,
      data: [
        1800, 2400, 2200, 3100, 4700, 5600, 6100, 7400, 6900, 7800, 8400, 9200,
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

</script>

<template>
  <div class="space-y-6 p-4">
    <section class="rounded-xl border border-gray-200 bg-white p-6">
      <div class="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
        <div class="max-w-3xl">
          <p class="text-sm font-semibold text-green-600">PowerX AgentOS</p>
          <h1 class="mt-2 text-2xl font-semibold text-gray-950">
            企业插件与 AI Agent 的统一运行底座
          </h1>
          <p class="mt-3 text-sm leading-6 text-gray-600">
            统一管理插件运行时、Integration Gateway、Runtime Scheduler、AI Engine 与 Knowledge Space，
            让 SCRM、电商和营销工具插件共享 IAM、权限、调度、通知和可观测能力。
          </p>
        </div>
        <div class="grid min-w-[320px] grid-cols-3 gap-3">
          <div
            v-for="item in pluginPortfolio"
            :key="item.name"
            class="rounded-lg border border-gray-200 bg-gray-50 p-3"
          >
            <p class="text-sm font-semibold text-gray-900">{{ item.name }}</p>
            <p class="mt-1 text-xs font-medium text-green-600">{{ item.status }}</p>
            <p class="mt-2 text-xs leading-5 text-gray-500">{{ item.desc }}</p>
          </div>
        </div>
      </div>
    </section>

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
              <span class="text-sm text-gray-500 ml-1">当前状态</span>
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

    <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
      <UCard v-for="module in platformModules" :key="module.name">
        <div class="flex items-start gap-3">
          <div class="rounded-lg bg-gray-50 p-2">
            <UIcon :name="module.icon" class="inline-block h-5 w-5 text-green-600" />
          </div>
          <div>
            <div class="flex items-center gap-2">
              <h3 class="text-sm font-semibold text-gray-900">{{ module.name }}</h3>
              <span class="rounded-full bg-green-50 px-2 py-0.5 text-xs font-medium text-green-700">
                {{ module.status }}
              </span>
            </div>
            <p class="mt-2 text-xs leading-5 text-gray-500">{{ module.desc }}</p>
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
              <h3 class="text-lg font-semibold text-gray-900">底座能力调用趋势</h3>
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
            <h3 class="text-lg font-semibold text-gray-900">最近运行事件</h3>
          </template>

          <div class="space-y-4">
            <div
              v-for="event in runtimeEvents"
              :key="event.title"
              class="flex items-start space-x-3"
            >
              <div
                class="w-8 h-8 rounded-full flex items-center justify-center text-xs font-medium text-white"
                :class="{
                  'bg-green-500': event.tone === 'green',
                  'bg-blue-500': event.tone === 'blue',
                  'bg-emerald-500': event.tone === 'emerald',
                }"
              >
                <UIcon name="i-heroicons-check" class="h-4 w-4" />
              </div>
              <div class="flex-1 min-w-0">
                <p class="text-sm font-medium text-gray-900">{{ event.title }}</p>
                <p class="mt-1 text-xs leading-5 text-gray-500">{{ event.desc }}</p>
                <p class="text-xs text-gray-400 mt-1">{{ event.time }}</p>
              </div>
            </div>
          </div>

          <template #footer>
            <UButton variant="ghost" block> 打开 Monitor Center </UButton>
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
