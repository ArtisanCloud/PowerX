<template>
  <div class="workflow-editor" :class="{ dark: isDark }">
    <!-- 工具栏 -->
    <div class="workflow-toolbar">
      <UButton
        icon="i-heroicons-arrow-uturn-left"
        color="neutral"
        variant="ghost"
        size="sm"
        class="gap-2 whitespace-nowrap"
        @click="undo"
        :disabled="!canUndo"
      >
        撤销
      </UButton>
      <UButton
        icon="i-heroicons-arrow-uturn-right"
        color="neutral"
        variant="ghost"
        size="sm"
        class="gap-2 whitespace-nowrap"
        @click="redo"
        :disabled="!canRedo"
      >
        重做
      </UButton>
      <USeparator vertical />
      <UButton
        icon="i-heroicons-plus"
        color="primary"
        size="sm"
        class="gap-2 whitespace-nowrap"
        @click="handleFitView"
      >
        适应视图
      </UButton>
      <UButton
        icon="i-heroicons-document-plus"
        color="primary"
        variant="ghost"
        size="sm"
        class="gap-2 whitespace-nowrap"
        @click="handleSaveWorkflow"
      >
        保存
      </UButton>
      <div class="flex-grow"></div>
      <UButton
        icon="i-heroicons-play"
        color="success"
        size="sm"
        class="gap-2 whitespace-nowrap"
        @click="runWorkflow"
      >
        运行
      </UButton>
    </div>

    <!-- 主编辑区域 -->
    <div class="workflow-main">
      <!-- 左侧节点面板 -->
      <div class="workflow-palette">
        <h3 class="palette-title">节点清单</h3>
        <div class="palette-search">
          <UInput
            v-model="paletteSearch"
            icon="i-heroicons-magnifying-glass"
            placeholder="搜索节点..."
            :color="isDark ? 'gray' : 'white'"
          />
        </div>
        <div class="palette-items">
          <div
            v-for="item in filteredPalette"
            :key="item.id"
            class="palette-item"
            draggable="true"
            @dragstart="onDragStart($event, item.id)"
          >
            <div class="palette-item-icon">
              <Icon :name="item.icon || 'i-heroicons-square-2-stack'" />
            </div>
            <div class="palette-item-content">
              <div class="palette-item-title">{{ item.label }}</div>
              <div class="palette-item-kind">{{ getKindLabel(item.kind) }}</div>
            </div>
          </div>
        </div>
      </div>

      <!-- 中间画布区域 -->
      <div class="workflow-canvas vue-flow-wrapper">
        <VueFlow
          v-model:nodes="nodes"
          v-model:edges="edges"
          :default-viewport="{ x: 0, y: 0, zoom: 1 }"
          :min-zoom="0.2"
          :max-zoom="4"
          class="workflow-vue-flow"
          @drop="onDrop"
          @dragover="onDragOver"
          @connect="handleConnect"
          @node-drag-stop="onNodeDragStop"
          @pane-ready="onReady"
          @node-click="onNodeClick"
        >
          <template #node-generic="nodeProps">
            <GenericNode
              v-bind="nodeProps"
              @update:props="updateNodeProps(nodeProps.id, $event)"
            />
          </template>

          <Background pattern-color="#aaa" :gap="8" />
          <MiniMap />
          <Controls />
        </VueFlow>
      </div>

      <!-- 右侧属性面板 -->
      <div class="workflow-properties" v-if="selectedNode">
        <h3 class="properties-title">节点属性</h3>
        <div class="properties-content">
          <div class="properties-header">
            <h4>{{ selectedNode.data.label }}</h4>
            <div class="properties-kind">
              {{ getKindLabel(selectedNode.data.kind) }}
            </div>
          </div>

          <USeparator />

          <div class="properties-form">
            <template
              v-for="(value, key) in selectedNode.data.props"
              :key="key"
            >
              <div class="properties-field">
                <UForm :label="key">
                  <!-- 根据属性类型渲染不同的输入控件 -->
                  <template v-if="typeof value === 'boolean'">
                    <USwitch v-model="selectedNode.data.props[key]" />
                  </template>
                  <template v-else-if="typeof value === 'number'">
                    <UInput
                      v-model.number="selectedNode.data.props[key]"
                      type="number"
                      :step="getNumberStep(key, selectedNode.data.schema)"
                    />
                  </template>
                  <template
                    v-else-if="isEnumField(key, selectedNode.data.schema)"
                  >
                    <USelect
                      v-model="selectedNode.data.props[key]"
                      :items="getEnumOptions(key, selectedNode.data.schema)"
                    />
                  </template>
                  <template
                    v-else-if="typeof value === 'string' && value.length > 50"
                  >
                    <UTextarea
                      v-model="selectedNode.data.props[key]"
                      :rows="5"
                    />
                  </template>
                  <template
                    v-else-if="typeof value === 'object' && value !== null"
                  >
                    <UTextarea
                      v-model="objectProps[key]"
                      :rows="5"
                      @blur="updateObjectProp(key)"
                    />
                  </template>
                  <template v-else>
                    <UInput v-model="selectedNode.data.props[key]" />
                  </template>
                </UForm>
              </div>
            </template>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, reactive } from "vue";
import { useColorMode } from "#imports";
import { VueFlow, useVueFlow } from "@vue-flow/core";
import { Background } from "@vue-flow/background";
import { Controls } from "@vue-flow/controls";
import { MiniMap } from "@vue-flow/minimap";
import type { Node, Edge, Connection } from "@vue-flow/core";
import type { KindSpec, PaletteItem } from "~/types/workflow";

import "@vue-flow/core/dist/style.css";
import "@vue-flow/core/dist/theme-default.css";
import "@vue-flow/controls/dist/style.css";
import "@vue-flow/minimap/dist/style.css";

import "@vue-flow/core/dist/style.css";
import "@vue-flow/core/dist/theme-default.css";
import "@vue-flow/controls/dist/style.css";
import "@vue-flow/minimap/dist/style.css";
import { useWorkflowManager } from "~/composables/workflow/useWorkflowManager";
import GenericNode from "./nodes/GenericNode.vue";

// 主题支持
const colorMode = useColorMode();
const isDark = computed(() => colorMode.value === "dark");

// 工作流管理器
const {
  kinds,
  palette,
  currentWorkflow,
  loadKinds,
  loadPalette,
  addNodeFromPalette,
  saveWorkflow,
} = useWorkflowManager();

// Vue Flow 实例
const {
  findNode,
  getNodes,
  getEdges,
  addEdges,
  setViewport,
  fitView,
  project,
  addNodes,
} = useVueFlow();

// 状态
const nodes = ref<Node[]>([]);
const edges = ref<Edge[]>([]);
const paletteSearch = ref("");
const selectedNode = ref<Node | null>(null);
const objectProps = reactive<Record<string, string>>({});

// 过滤后的节点清单
const filteredPalette = computed(() => {
  if (!paletteSearch.value) return palette.value;

  const search = paletteSearch.value.toLowerCase();
  return palette.value.filter(
    (item) =>
      item.label.toLowerCase().includes(search) ||
      item.kind.toLowerCase().includes(search)
  );
});

// 获取节点类型标签
function getKindLabel(kind: string) {
  return kinds.value[kind]?.label || kind;
}

// 拖拽开始
function onDragStart(event: DragEvent, paletteId: string) {
  if (event.dataTransfer) {
    event.dataTransfer.setData("application/vueflow", paletteId);
    event.dataTransfer.effectAllowed = "move";
  }
}

// 拖拽放置
function onDrop(event: DragEvent) {
  if (!event.dataTransfer) return;

  const paletteId = event.dataTransfer.getData("application/vueflow");
  if (!paletteId) return;

  // 获取放置位置
  const position = project({
    x: event.clientX,
    y: event.clientY,
  });

  // 添加节点
  const newNode = addNodeFromPalette(paletteId, position);
  if (newNode) {
    addNodes([newNode]);
  }
}

// 适应视图
function handleFitView() {
  fitView();
}

// 保存工作流
async function handleSaveWorkflow() {
  await saveWorkflow(nodes.value, edges.value);
}

// 撤销/重做功能（简单实现）
const canUndo = ref(false);
const canRedo = ref(false);

function undo() {
  // 简单的撤销实现
  console.log("撤销操作");
}

function redo() {
  // 简单的重做实现
  console.log("重做操作");
}

// 允许拖拽
function onDragOver(event: DragEvent) {
  if (event.preventDefault) {
    event.preventDefault();
  }

  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = "move";
  }
}

// 连接节点
function handleConnect(params: Connection) {
  console.log("连接节点:", params);
  addEdges([params]);
}

// 节点拖拽结束
function onNodeDragStop() {
  // Vue Flow 会自动更新节点位置
}

// 画布准备完成
function onReady() {
  fitView();
}

// 点击节点
function onNodeClick(event: { node: Node }) {
  const node = event.node;
  selectedNode.value = node;

  // 初始化对象属性编辑器
  if (node.data.props) {
    for (const [key, value] of Object.entries(node.data.props)) {
      if (typeof value === "object" && value !== null) {
        objectProps[key] = JSON.stringify(value, null, 2);
      }
    }
  }
}

// 更新节点属性
function updateNodeProps(nodeId: string, newProps: Record<string, any>) {
  const node = findNode(nodeId);
  if (node && node.data) {
    node.data.props = { ...node.data.props, ...newProps };
  }
}

// 更新对象属性
function updateObjectProp(key: string) {
  if (!selectedNode.value) return;

  try {
    selectedNode.value.data.props[key] = JSON.parse(objectProps[key]);
  } catch (err) {
    console.error(`无法解析JSON: ${objectProps[key]}`, err);
    // 恢复原始值
    objectProps[key] = JSON.stringify(
      selectedNode.value.data.props[key],
      null,
      2
    );
  }
}

// 判断是否为枚举字段
function isEnumField(key: string, schema: any) {
  if (!schema?.properties?.[key]?.enum) return false;
  return Array.isArray(schema.properties[key].enum);
}

// 获取枚举选项
function getEnumOptions(key: string, schema: any) {
  if (!schema?.properties?.[key]?.enum) return [];
  return schema.properties[key].enum.map((value: any) => ({
    label: value,
    value,
  }));
}

// 获取数字输入步长
function getNumberStep(key: string, schema: any) {
  if (!schema?.properties?.[key]) return 1;

  const prop = schema.properties[key];
  if (prop.type !== "number") return 1;

  // 如果有最小和最大值，计算合适的步长
  if (prop.minimum !== undefined && prop.maximum !== undefined) {
    const range = prop.maximum - prop.minimum;
    if (range <= 1) return 0.1;
    if (range <= 10) return 0.5;
    if (range <= 100) return 1;
    return Math.pow(10, Math.floor(Math.log10(range)) - 2);
  }

  return 1;
}

// 运行工作流
function runWorkflow() {
  // 这里应该调用API运行工作流
  console.log("运行工作流", currentWorkflow.value?.id);
}

// 注意：在当前版本的 Vue Flow 中，我们通过点击事件来处理节点选择
// 不需要监听 getSelectedNodes，因为我们在 onNodeClick 中已经处理了选择逻辑

// 组件挂载
onMounted(async () => {
  await loadKinds();
  await loadPalette();
});
</script>

<style scoped>
.workflow-editor {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  background-color: var(--bg-primary);
  color: var(--text-primary);
}

.workflow-toolbar {
  display: flex;
  align-items: center;
  padding: 8px 16px;
  border-bottom: 1px solid var(--border-color);
  gap: 12px;
  background-color: var(--bg-secondary);
}

.flex-grow {
  flex-grow: 1;
}

.workflow-main {
  display: flex;
  flex: 1;
  overflow: hidden;
  background-color: var(--bg-primary);
}

.workflow-palette {
  width: 240px;
  border-right: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background-color: var(--bg-primary);
}

.palette-title {
  padding: 12px 16px;
  font-weight: 600;
  font-size: 16px;
  border-bottom: 1px solid var(--border-color);
  background-color: var(--bg-secondary);
  color: var(--text-primary);
}

.palette-search {
  padding: 8px 16px;
  border-bottom: 1px solid var(--border-color);
}

.palette-items {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.palette-item {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  border-radius: 6px;
  cursor: grab;
  margin-bottom: 8px;
  background-color: var(--bg-secondary);
  border: 1px solid var(--border-color);
  transition: all 0.2s;
  color: var(--text-primary);
}

.palette-item:hover {
  background-color: var(--hover-bg);
  border-color: var(--border-color);
}

.palette-item-icon {
  margin-right: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background-color: var(--border-color);
  border-radius: 6px;
}

.palette-item-content {
  flex: 1;
}

.palette-item-title {
  font-weight: 500;
  font-size: 14px;
}

.palette-item-kind {
  font-size: 12px;
  color: var(--text-secondary);
}

.workflow-canvas {
  flex: 1;
  position: relative;
  overflow: hidden;
  background-color: var(--bg-secondary);
}

.workflow-vue-flow {
  width: 100%;
  height: 100%;
}

.workflow-properties {
  width: 300px;
  border-left: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background-color: var(--bg-primary);
}

.properties-title {
  padding: 12px 16px;
  font-weight: 600;
  font-size: 16px;
  border-bottom: 1px solid var(--border-color);
  background-color: var(--bg-secondary);
  color: var(--text-primary);
}

.properties-content {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
}

.properties-header {
  margin-bottom: 16px;
}

.properties-header h4 {
  font-weight: 600;
  font-size: 16px;
  margin: 0 0 4px 0;
  color: var(--text-primary);
}

.properties-kind {
  font-size: 12px;
  color: var(--text-secondary);
}

.properties-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.properties-field {
  margin-bottom: 8px;
}

/* Vue Flow 主题支持 */
.vue-flow__background {
  background-color: var(--bg-secondary) !important;
}

.vue-flow__minimap {
  background-color: var(--card-bg) !important;
  border-color: var(--border-color) !important;
}

.vue-flow__controls {
  background-color: var(--card-bg) !important;
  border-color: var(--border-color) !important;
}

.vue-flow__controls button {
  background-color: var(--bg-secondary) !important;
  color: var(--text-primary) !important;
  border-color: var(--border-color) !important;
}

.vue-flow__controls button:hover {
  background-color: var(--hover-bg) !important;
}
</style>
