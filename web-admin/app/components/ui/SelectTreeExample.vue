<!-- SelectTree 使用示例 -->
<script setup lang="ts">
import { ref } from "vue";
import type { TreeItemBase } from "~/composables/api/types/tree";

// 示例数据
const selectedValue = ref<string | null>(null);

// 使用TreeItemBase类型，将原有数据结构适配
const treeData: TreeItemBase[] = [
  {
    id: "org",
    title: "组织架构",
    icon: "i-heroicons-building-office",
    children: [
      {
        id: "tech",
        title: "技术部",
        icon: "i-heroicons-code-bracket",
        children: [
          {
            id: "frontend",
            title: "前端组",
            icon: "i-heroicons-computer-desktop",
          },
          { id: "backend", title: "后端组", icon: "i-heroicons-server" },
          { id: "qa", title: "测试组", icon: "i-heroicons-bug-ant" },
        ],
      },
      {
        id: "product",
        title: "产品部",
        icon: "i-heroicons-light-bulb",
        children: [
          { id: "pm", title: "产品经理", icon: "i-heroicons-user" },
          { id: "ui", title: "UI设计师", icon: "i-heroicons-paint-brush" },
        ],
      },
      {
        id: "operation",
        title: "运营部",
        icon: "i-heroicons-chart-bar",
        children: [
          {
            id: "marketing",
            title: "市场推广",
            icon: "i-heroicons-megaphone",
          },
          { id: "service", title: "客户服务", icon: "i-heroicons-phone" },
        ],
      },
    ],
  },
  {
    id: "external",
    title: "外部合作",
    icon: "i-heroicons-building-storefront",
    children: [
      { id: "supplier-a", title: "供应商A", icon: "i-heroicons-truck" },
      { id: "partner-b", title: "合作伙伴B", icon: "i-heroicons-handshake" },
    ],
  },
];

const regionData: TreeItemBase[] = [
  {
    id: "china",
    title: "中国",
    children: [
      {
        id: "north-china",
        title: "华北地区",
        children: [
          { id: "beijing", title: "北京市" },
          { id: "tianjin", title: "天津市" },
          { id: "hebei", title: "河北省" },
        ],
      },
      {
        id: "east-china",
        title: "华东地区",
        children: [
          { id: "shanghai", title: "上海市" },
          { id: "jiangsu", title: "江苏省" },
          { id: "zhejiang", title: "浙江省" },
        ],
      },
    ],
  },
  {
    id: "usa",
    title: "美国",
    children: [
      { id: "california", title: "加利福尼亚州" },
      { id: "newyork", title: "纽约州" },
    ],
  },
];

// 事件处理
const handleChange = (value: string | null) => {
  console.log("选择变化:", value);
};

const handleClear = () => {
  console.log("已清除选择");
};
</script>

<template>
  <div class="space-y-8 p-6">
    <div>
      <h2 class="text-xl font-semibold mb-4">SelectTree 组件示例</h2>

      <!-- 基础用法 -->
      <div class="space-y-4">
        <div>
          <h3 class="text-lg font-medium mb-2">基础用法</h3>
          <SelectTree
            v-model="selectedValue"
            :items="treeData"
            placeholder="请选择部门"
            @change="handleChange"
            @clear="handleClear"
          />
          <p class="text-sm text-gray-500 mt-2">
            选中值: {{ selectedValue || "未选择" }}
          </p>
        </div>

        <!-- 可清除 -->
        <div>
          <h3 class="text-lg font-medium mb-2">可清除</h3>
          <SelectTree
            v-model="selectedValue"
            :items="treeData"
            placeholder="请选择部门"
            clearable
            @change="handleChange"
            @clear="handleClear"
          />
        </div>

        <!-- 可搜索 -->
        <div>
          <h3 class="text-lg font-medium mb-2">可搜索</h3>
          <SelectTree
            v-model="selectedValue"
            :items="regionData"
            placeholder="请选择地区"
            searchable
            clearable
            @change="handleChange"
          />
        </div>

        <!-- 不同尺寸 -->
        <div>
          <h3 class="text-lg font-medium mb-2">不同尺寸</h3>
          <div class="flex gap-4 items-center">
            <SelectTree
              v-model="selectedValue"
              :items="treeData"
              placeholder="小尺寸"
              size="sm"
            />
            <SelectTree
              v-model="selectedValue"
              :items="treeData"
              placeholder="中等尺寸"
              size="md"
            />
            <SelectTree
              v-model="selectedValue"
              :items="treeData"
              placeholder="大尺寸"
              size="lg"
            />
          </div>
        </div>

        <!-- 不同样式 -->
        <div>
          <h3 class="text-lg font-medium mb-2">不同样式</h3>
          <div class="flex gap-4 items-center">
            <SelectTree
              v-model="selectedValue"
              :items="treeData"
              placeholder="outline"
              variant="outline"
            />
            <SelectTree
              v-model="selectedValue"
              :items="treeData"
              placeholder="subtle"
              variant="subtle"
            />
            <SelectTree
              v-model="selectedValue"
              :items="treeData"
              placeholder="ghost"
              variant="ghost"
            />
          </div>
        </div>

        <!-- 禁用状态 -->
        <div>
          <h3 class="text-lg font-medium mb-2">禁用状态</h3>
          <SelectTree
            v-model="selectedValue"
            :items="treeData"
            placeholder="已禁用"
            disabled
          />
        </div>

        <!-- 加载状态 -->
        <div>
          <h3 class="text-lg font-medium mb-2">加载状态</h3>
          <SelectTree
            v-model="selectedValue"
            :items="treeData"
            placeholder="加载中..."
            loading
          />
        </div>

        <!-- 自定义样式 -->
        <div>
          <h3 class="text-lg font-medium mb-2">自定义样式</h3>
          <SelectTree
            v-model="selectedValue"
            :items="treeData"
            placeholder="自定义样式"
            icon="i-heroicons-building-office"
            color="primary"
            tree-class="w-80 max-h-80"
            button-class="min-w-[200px]"
          />
        </div>
      </div>
    </div>
  </div>
</template>
