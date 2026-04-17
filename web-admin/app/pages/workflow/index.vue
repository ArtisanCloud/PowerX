<template>
  <div class="workflow-page">
    <!-- 页面头部 -->
    <div class="workflow-header">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 flex items-center">
            <UIcon name="i-heroicons-squares-2x2" class="mr-2" />
            工作流管理
          </h1>
          <p class="text-gray-600 mt-1">创建和管理您的自动化工作流</p>
        </div>
        <div class="flex items-center space-x-3">
          <UInput
            v-model="searchQuery"
            placeholder="搜索工作流..."
            icon="i-heroicons-magnifying-glass"
            class="w-64"
          />
          <UButton
            icon="i-heroicons-document-plus"
            color="primary"
            @click="createNewWorkflow"
          >
            新建工作流
          </UButton>
        </div>
      </div>
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-6 mb-6">
      <UCard>
        <div class="flex items-center">
          <div class="p-2 bg-blue-100 rounded-lg">
            <UIcon
              name="i-heroicons-squares-2x2"
              class="w-6 h-6 text-blue-600"
            />
          </div>
          <div class="ml-4">
            <p class="text-sm font-medium text-gray-600">总工作流</p>
            <p class="text-2xl font-bold text-gray-900">{{ stats.total }}</p>
          </div>
        </div>
      </UCard>

      <UCard>
        <div class="flex items-center">
          <div class="p-2 bg-green-100 rounded-lg">
            <UIcon name="i-heroicons-play" class="w-6 h-6 text-green-600" />
          </div>
          <div class="ml-4">
            <p class="text-sm font-medium text-gray-600">运行中</p>
            <p class="text-2xl font-bold text-gray-900">{{ stats.active }}</p>
          </div>
        </div>
      </UCard>

      <UCard>
        <div class="flex items-center">
          <div class="p-2 bg-yellow-100 rounded-lg">
            <UIcon name="i-heroicons-pause" class="w-6 h-6 text-yellow-600" />
          </div>
          <div class="ml-4">
            <p class="text-sm font-medium text-gray-600">已暂停</p>
            <p class="text-2xl font-bold text-gray-900">{{ stats.paused }}</p>
          </div>
        </div>
      </UCard>

      <UCard>
        <div class="flex items-center">
          <div class="p-2 bg-purple-100 rounded-lg">
            <UIcon name="i-heroicons-clock" class="w-6 h-6 text-purple-600" />
          </div>
          <div class="ml-4">
            <p class="text-sm font-medium text-gray-600">本月执行</p>
            <p class="text-2xl font-bold text-gray-900">
              {{ stats.executions }}
            </p>
          </div>
        </div>
      </UCard>
    </div>

    <!-- 工作流列表 -->
    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold">工作流列表</h2>
          <div class="flex items-center space-x-2">
            <USelectMenu
              v-model="statusFilter"
              :items="statusOptions"
              placeholder="状态筛选"
              class="w-32"
            />
            <UButton
              icon="i-heroicons-arrow-path"
              variant="ghost"
              @click="refreshWorkflows"
            />
          </div>
        </div>
      </template>

      <!-- 空状态 -->
      <div
        v-if="filteredWorkflows.length === 0 && !loading"
        class="text-center py-12"
      >
        <div
          class="mx-auto w-24 h-24 bg-gray-100 rounded-full flex items-center justify-center mb-4"
        >
          <UIcon
            name="i-heroicons-squares-2x2"
            class="w-12 h-12 text-gray-400"
          />
        </div>
        <h3 class="text-lg font-medium text-gray-900 mb-2">暂无工作流</h3>
        <p class="text-gray-500 mb-6">创建您的第一个工作流来开始自动化</p>
        <UButton
          icon="i-heroicons-document-plus"
          color="primary"
          @click="createNewWorkflow"
        >
          创建工作流
        </UButton>
      </div>

      <!-- 工作流网格 -->
      <div v-else>
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          <div
            v-for="workflow in pagedWorkflows"
            :key="workflow.id"
            class="border border-gray-200 rounded-lg p-6 hover:shadow-md transition-shadow cursor-pointer"
            @click="openWorkflowEditor(workflow.id)"
          >
            <div class="flex items-start justify-between mb-4">
              <div class="flex items-center">
                <div
                  class="w-10 h-10 rounded-lg flex items-center justify-center"
                  :class="getStatusColor(workflow.status)"
                >
                  <UIcon name="i-heroicons-squares-2x2" class="w-5 h-5" />
                </div>
                <div class="ml-3">
                  <h3 class="font-semibold text-gray-900">
                    {{ workflow.name }}
                  </h3>
                  <p class="text-sm text-gray-500">
                    {{ workflow.description || "无描述" }}
                  </p>
                </div>
              </div>
              <UDropdownMenu :items="getWorkflowActions(workflow)" @click.stop>
                <UButton
                  icon="i-heroicons-ellipsis-vertical"
                  variant="ghost"
                  size="sm"
                />
              </UDropdownMenu>
            </div>

            <div
              class="flex items-center justify-between text-sm text-gray-500 mb-3"
            >
              <span>{{ formatDate(workflow.updatedAt) }}</span>
              <UBadge
                :color="getStatusBadgeColor(workflow.status)"
                variant="soft"
              >
                {{ getStatusText(workflow.status) }}
              </UBadge>
            </div>

            <div
              class="flex items-center justify-between text-xs text-gray-400"
            >
              <span>{{ workflow.nodeCount || 0 }} 个节点</span>
              <span>执行 {{ workflow.executionCount || 0 }} 次</span>
            </div>
          </div>
        </div>

        <!-- 分页条 -->
        <div class="mt-6 flex items-center justify-between">
          <div class="text-sm text-gray-500 flex items-center gap-2">
            <span>共 {{ total }} 条</span>
            <span>·</span>
            <USelectMenu
              v-model="pageCount"
              :options="pageCountOptions"
              class="w-32"
            />
          </div>

          <!-- max 控制显示的页码按钮数量 -->
          <UPagination
            v-model:page="page"
            :total="total"
            :page-count="pageCount"
            :max="5"
          />
        </div>
      </div>
    </UCard>

    <!-- 新建工作流对话框 -->
    <UModal
      v-model="showNewWorkflowModal"
      title="workflow-title"
      description="workflow-desc"
    >
      <template #content>
        <UCard>
          <template #header>
            <div class="flex items-center">
              <UIcon name="i-heroicons-document-plus" class="mr-2" />
              新建工作流
            </div>
          </template>

          <div class="space-y-4">
            <UFormField label="工作流名称" required>
              <UInput v-model="newWorkflow.name" placeholder="输入工作流名称" />
            </UFormField>

            <UFormField label="描述">
              <UTextarea
                v-model="newWorkflow.description"
                placeholder="输入工作流描述（可选）"
              />
            </UFormField>
          </div>

          <template #footer>
            <div class="flex justify-end gap-2">
              <UButton
                color="gray"
                variant="ghost"
                @click="showNewWorkflowModal = false"
              >
                取消
              </UButton>
              <UButton
                color="primary"
                :disabled="!newWorkflow.name"
                @click="confirmCreateWorkflow"
              >
                创建
              </UButton>
            </div>
          </template>
        </UCard>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, reactive } from "vue";
import { useWorkflowManager } from "~/composables/workflow/useWorkflowManager";

// 工作流管理器
const { createNewWorkflow: createWf, getWorkflowList } = useWorkflowManager();

// 状态
const loading = ref(false);
const searchQuery = ref("");
const statusFilter = ref(null);
const showNewWorkflowModal = ref(false);
const newWorkflow = reactive({
  name: "",
  description: "",
});

// 工作流接口
interface WorkflowItem {
  id: string;
  name: string;
  description?: string;
  status: "active" | "paused" | "draft" | "error";
  updatedAt: string;
  nodeCount?: number;
  executionCount?: number;
}

// 模拟工作流数据
const workflowList = ref<WorkflowItem[]>([
  {
    id: "1",
    name: "客户数据同步",
    description: "自动同步客户数据到CRM系统",
    status: "active",
    updatedAt: "2024-01-15T10:30:00Z",
    nodeCount: 8,
    executionCount: 156,
  },
  {
    id: "2",
    name: "订单处理流程",
    description: "处理新订单并发送确认邮件",
    status: "active",
    updatedAt: "2024-01-14T16:45:00Z",
    nodeCount: 12,
    executionCount: 89,
  },
  {
    id: "3",
    name: "库存预警",
    description: "监控库存水平并发送预警通知",
    status: "paused",
    updatedAt: "2024-01-13T09:15:00Z",
    nodeCount: 6,
    executionCount: 234,
  },
  {
    id: "4",
    name: "用户注册流程",
    description: "处理新用户注册和欢迎邮件",
    status: "draft",
    updatedAt: "2024-01-12T14:20:00Z",
    nodeCount: 4,
    executionCount: 0,
  },
]);

// 统计数据
const stats = computed(() => ({
  total: workflowList.value.length,
  active: workflowList.value.filter((w) => w.status === "active").length,
  paused: workflowList.value.filter((w) => w.status === "paused").length,
  executions: workflowList.value.reduce(
    (sum, w) => sum + (w.executionCount || 0),
    0
  ),
}));

// 状态筛选选项
const statusOptions = [
  { label: "全部", value: "" },
  { label: "运行中", value: "active" },
  { label: "已暂停", value: "paused" },
  { label: "草稿", value: "draft" },
  { label: "错误", value: "error" },
];

// 过滤后的工作流
const filteredWorkflows = computed(() => {
  let filtered = workflowList.value;

  if (searchQuery.value) {
    filtered = filtered.filter(
      (w) =>
        w.name.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
        w.description?.toLowerCase().includes(searchQuery.value.toLowerCase())
    );
  }

  if (statusFilter.value) {
    filtered = filtered.filter((w) => w.status === statusFilter.value);
  }

  return filtered;
});

// —— 分页 ——
// 当前页（从 1 开始）
const page = ref(1);
// 每页数量（可做成可选）
const pageCount = ref(9);
// 可选：每页下拉选项
const pageCountOptions = [
  { label: "每页 6 条", value: 6 },
  { label: "每页 9 条", value: 9 },
  { label: "每页 12 条", value: 12 },
  { label: "每页 18 条", value: 18 },
  { label: "每页 24 条", value: 24 },
];

// 总条数
const total = computed(() => filteredWorkflows.value.length);

// 当前页数据（渲染用）
const pagedWorkflows = computed(() => {
  const start = (page.value - 1) * pageCount.value;
  return filteredWorkflows.value.slice(start, start + pageCount.value);
});

// 搜索/筛选变化时回到第 1 页
watch([searchQuery, statusFilter], () => {
  page.value = 1;
});

// 当每页条数改变或过滤结果变短，确保当前页不越界
watch([() => filteredWorkflows.value.length, pageCount], () => {
  const maxPage = Math.max(1, Math.ceil(total.value / pageCount.value));
  if (page.value > maxPage) page.value = maxPage;
});

// 创建新工作流
function createNewWorkflow() {
  showNewWorkflowModal.value = true;
}

// 确认创建工作流
async function confirmCreateWorkflow() {
  if (!newWorkflow.name) return;

  const workflow = await createWf(newWorkflow.name, newWorkflow.description);
  if (workflow) {
    showNewWorkflowModal.value = false;
    newWorkflow.name = "";
    newWorkflow.description = "";
    await navigateTo(`/workflow/workspace?id=${workflow.id}`);
  }
}

// 打开工作流编辑器
function openWorkflowEditor(id: string) {
  navigateTo(`/workflow/workspace?id=${id}`);
}

// 刷新工作流列表
async function refreshWorkflows() {
  loading.value = true;
  try {
    // 这里可以调用实际的API
    await new Promise((resolve) => setTimeout(resolve, 1000));
  } finally {
    loading.value = false;
  }
}

// 获取状态颜色
function getStatusColor(status: string) {
  const colors = {
    active: "bg-green-100 text-green-600",
    paused: "bg-yellow-100 text-yellow-600",
    draft: "bg-gray-100 text-gray-600",
    error: "bg-red-100 text-red-600",
  };
  return colors[status as keyof typeof colors] || colors.draft;
}

// 获取状态徽章颜色
function getStatusBadgeColor(status: string) {
  const colors = {
    active: "green",
    paused: "yellow",
    draft: "gray",
    error: "red",
  };
  return colors[status as keyof typeof colors] || "gray";
}

// 获取状态文本
function getStatusText(status: string) {
  const texts = {
    active: "运行中",
    paused: "已暂停",
    draft: "草稿",
    error: "错误",
  };
  return texts[status as keyof typeof texts] || "未知";
}

// 格式化日期
function formatDate(dateString: string) {
  return new Date(dateString).toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

// 获取工作流操作菜单
function getWorkflowActions(workflow: WorkflowItem) {
  return [
    [
      {
        label: "编辑",
        icon: "i-heroicons-pencil",
        click: () => openWorkflowEditor(workflow.id),
      },
      {
        label: workflow.status === "active" ? "暂停" : "启动",
        icon:
          workflow.status === "active"
            ? "i-heroicons-pause"
            : "i-heroicons-play",
        click: () => toggleWorkflowStatus(workflow.id),
      },
    ],
    [
      {
        label: "复制",
        icon: "i-heroicons-document-duplicate",
        click: () => duplicateWorkflow(workflow.id),
      },
      {
        label: "导出",
        icon: "i-heroicons-arrow-down-tray",
        click: () => exportWorkflow(workflow.id),
      },
    ],
    [
      {
        label: "删除",
        icon: "i-heroicons-trash",
        click: () => deleteWorkflow(workflow.id),
      },
    ],
  ];
}

// 切换工作流状态
function toggleWorkflowStatus(id: string) {
  const workflow = workflowList.value.find((w) => w.id === id);
  if (workflow) {
    workflow.status = workflow.status === "active" ? "paused" : "active";
  }
}

// 复制工作流
function duplicateWorkflow(id: string) {
  console.info("复制工作流:", id);
}

// 导出工作流
function exportWorkflow(id: string) {
  console.info("导出工作流:", id);
}

// 删除工作流
function deleteWorkflow(id: string) {
  const index = workflowList.value.findIndex((w) => w.id === id);
  if (index > -1) {
    workflowList.value.splice(index, 1);
  }
}

// 组件挂载
onMounted(() => {
  refreshWorkflows();
});
</script>

<style scoped>
.workflow-page {
  padding: 24px;
  max-width: 1200px;
  margin: 0 auto;
}

.workflow-header {
  margin-bottom: 24px;
}
</style>
