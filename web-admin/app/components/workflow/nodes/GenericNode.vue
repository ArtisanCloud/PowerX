<template>
  <div 
    :class="[
      'wf-node', 
      `wf-node-${nodeData.ui?.shape || 'card'}`, 
      `wf-node-${nodeData.ui?.colorToken || 'default'}`,
      nodeData.runState ? `wf-node-run-${nodeData.runState}` : '',
      { selected }
    ]"
    :style="nodeStyle"
  >
    <!-- 输入端口 -->
    <template v-for="port in (nodeData.ports?.inputs || [])" :key="`in-${port.name}`">
      <Handle
        :id="port.name"
        type="target"
        :position="getHandlePosition('in', port.name)"
        :style="getHandleStyle('in', port.name)"
        class="wf-node-handle wf-node-handle-in"
      />
    </template>

    <!-- 输出端口 -->
    <template v-for="port in (nodeData.ports?.outputs || [])" :key="`out-${port.name}`">
      <Handle
        :id="port.name"
        type="source"
        :position="getHandlePosition('out', port.name)"
        :style="getHandleStyle('out', port.name)"
        class="wf-node-handle wf-node-handle-out"
      />
    </template>

    <!-- 节点头部 -->
    <div class="wf-node-header">
      <div class="wf-node-icon" v-if="nodeData.ui?.icon">
        <Icon :name="nodeData.ui.icon" />
      </div>
      <div class="wf-node-title">{{ displayLabel(nodeData.label) }}</div>
      <div class="wf-node-badges" v-if="nodeData.runState || (nodeData.ui?.badges && nodeData.ui.badges.length)">
        <span
          v-if="nodeData.runState"
          class="wf-node-run-badge"
          :class="`state-${nodeData.runState}`"
        >
          {{ t(`workflow.state.${nodeData.runState}`) }}
        </span>
        <span 
          v-for="badge in nodeData.ui.badges" 
          :key="badge" 
          class="wf-node-badge"
        >
          {{ badge }}
        </span>
      </div>
    </div>

    <!-- 节点内容 -->
    <div class="wf-node-content">
      <!-- 如果有自定义组件，则使用自定义组件 -->
      <component 
        v-if="nodeData.ui?.component && registeredComponents[nodeData.ui.component]" 
        :is="registeredComponents[nodeData.ui.component]"
        :node-data="nodeData"
        @update:props="updateProps"
      />
      <!-- 否则使用默认预览模板 -->
      <div v-else-if="nodeData.ui?.previewTpl" class="wf-node-preview">
        {{ renderPreviewTemplate(nodeData.ui.previewTpl, nodeData.props) }}
      </div>
      <!-- 最简单的情况，显示属性数量 -->
      <div v-else class="wf-node-props-count">
        {{ t("workflow.editor.propertyCount", { count: Object.keys(nodeData.props || {}).length }) }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject } from 'vue';
import { useI18n } from '#imports';
import { Handle, Position } from '@vue-flow/core';
import type { NodeData } from '~/types/workflow';

const props = defineProps<{
  id: string;
  data: NodeData;
  selected: boolean;
}>();

const { t, te } = useI18n();

// 注册的自定义组件
const registeredComponents = inject('workflowComponents', {} as Record<string, any>);

// 节点数据
const nodeData = computed(() => props.data);

function displayLabel(label: string) {
  return te(label) ? t(label) : label;
}

// 节点样式
const nodeStyle = computed(() => {
  const size = nodeData.value.ui?.size;
  return {
    width: size?.w ? `${size.w}px` : 'auto',
    minWidth: '150px',
    minHeight: size?.h ? `${size.h}px` : 'auto',
  };
});

// 获取 Handle 位置
function getHandlePosition(type: 'in' | 'out', portName: string): Position {
  const handles = nodeData.value.ui?.handles || {};
  
  // 默认位置映射
  const defaultPositions = {
    in: Position.Left,
    out: Position.Right
  };
  
  // 查找端口在哪个位置
  let position = defaultPositions[type];
  
  for (const [pos, ports] of Object.entries(handles)) {
    if (Array.isArray(ports) && ports.includes(portName)) {
      switch (pos) {
        case 'top':
          position = Position.Top;
          break;
        case 'right':
          position = Position.Right;
          break;
        case 'bottom':
          position = Position.Bottom;
          break;
        case 'left':
          position = Position.Left;
          break;
      }
      break;
    }
  }
  
  return position;
}

// 获取 Handle 样式
function getHandleStyle(type: 'in' | 'out', portName: string) {
  const position = getHandlePosition(type, portName);
  
  // 根据位置调整样式
  const baseStyle = {
    width: '10px',
    height: '10px',
    background: '#555',
    border: '2px solid #fff',
  };
  
  switch (position) {
    case Position.Top:
      return { ...baseStyle, top: '-6px', left: '50%', transform: 'translateX(-50%)' };
    case Position.Right:
      return { ...baseStyle, right: '-6px', top: '50%', transform: 'translateY(-50%)' };
    case Position.Bottom:
      return { ...baseStyle, bottom: '-6px', left: '50%', transform: 'translateX(-50%)' };
    case Position.Left:
    default:
      return { ...baseStyle, left: '-6px', top: '50%', transform: 'translateY(-50%)' };
  }
}

// 渲染预览模板
function renderPreviewTemplate(template: string, props: Record<string, any>): string {
  return template.replace(/\{\{([^}]+)\}\}/g, (match, path) => {
    const keys = path.trim().split('.');
    let value = props;
    
    for (const key of keys) {
      if (value === undefined || value === null) return '';
      value = value[key];
    }
    
    return value !== undefined && value !== null ? String(value) : '';
  });
}

// 更新节点属性
function updateProps(newProps: Record<string, any>) {
  // 通知父组件更新属性
  emit('update:props', newProps);
}

const emit = defineEmits(['update:props']);
</script>

<style scoped>
.wf-node {
  position: relative;
  border-radius: 8px;
  background: #ffffff;
  box-shadow: 0 8px 22px rgba(15, 23, 42, 0.1);
  border: 2px solid #93c5fd;
  overflow: visible;
}

.wf-node.selected {
  border-color: #6366f1;
  box-shadow: 0 0 0 4px rgba(99, 102, 241, 0.18), 0 10px 24px rgba(15, 23, 42, 0.14);
}

/* 节点形状 */
.wf-node-card {
  border-radius: 8px;
}

.wf-node-diamond {
  transform: rotate(45deg);
}

.wf-node-diamond > * {
  transform: rotate(-45deg);
}

.wf-node-pill {
  border-radius: 24px;
}

.wf-node-minimal {
  box-shadow: none;
  background-color: transparent;
}

/* 颜色主题 */
.wf-node-primary {
  border-color: #3b82f6;
}

.wf-node-success {
  border-color: #10b981;
}

.wf-node-warning {
  border-color: #f59e0b;
}

.wf-node-info {
  border-color: #60a5fa;
}

.wf-node-danger {
  border-color: #ef4444;
}

/* 节点头部 */
.wf-node-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 12px;
  border-bottom: 1px solid #e5e7eb;
  background: #f8fafc;
  border-top-left-radius: 8px;
  border-top-right-radius: 8px;
}

.wf-node-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  flex: none;
  border-radius: 6px;
  background: #eff6ff;
  color: #2563eb;
}

.wf-node-title {
  flex: 1;
  font-weight: 500;
  font-size: 14px;
  color: #111827;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.wf-node-badges {
  display: flex;
  gap: 4px;
}

.wf-node-badge {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 999px;
  background-color: #eef2ff;
  color: #4b5563;
}

.wf-node-run-badge {
  border-radius: 999px;
  padding: 2px 7px;
  font-size: 10px;
  font-weight: 800;
  line-height: 1.2;
  white-space: nowrap;
}

.wf-node-run-badge.state-succeeded,
.wf-node-run-badge.state-completed {
  background: rgba(34, 197, 94, 0.14);
  color: #16a34a;
}

.wf-node-run-badge.state-running,
.wf-node-run-badge.state-queued,
.wf-node-run-badge.state-compensating {
  background: rgba(59, 130, 246, 0.16);
  color: #2563eb;
}

.wf-node-run-badge.state-waiting,
.wf-node-run-badge.state-pending {
  background: rgba(245, 158, 11, 0.16);
  color: #d97706;
}

.wf-node-run-badge.state-failed,
.wf-node-run-badge.state-canceled {
  background: rgba(239, 68, 68, 0.15);
  color: #dc2626;
}

.wf-node-run-succeeded,
.wf-node-run-completed {
  border-color: #22c55e;
  box-shadow: 0 0 0 3px rgba(34, 197, 94, 0.14), 0 12px 26px rgba(15, 23, 42, 0.12);
}

.wf-node-run-running,
.wf-node-run-queued,
.wf-node-run-compensating {
  border-color: #60a5fa;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.18), 0 12px 26px rgba(15, 23, 42, 0.12);
}

.wf-node-run-waiting,
.wf-node-run-pending {
  border-color: #f59e0b;
  box-shadow: 0 0 0 3px rgba(245, 158, 11, 0.16), 0 12px 26px rgba(15, 23, 42, 0.12);
}

.wf-node-run-failed,
.wf-node-run-canceled {
  border-color: #ef4444;
  box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.16), 0 12px 26px rgba(15, 23, 42, 0.12);
}

/* 节点内容 */
.wf-node-content {
  padding: 10px 12px;
  font-size: 12px;
  color: #4b5563;
}

.wf-node-preview {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-family: monospace;
}

.wf-node-props-count {
  color: #64748b;
  font-weight: 600;
}

/* Handle 样式 */
.wf-node-handle {
  position: absolute;
  border-radius: 50%;
  cursor: crosshair;
  z-index: 10;
}

.wf-node-handle:hover {
  background: #3b82f6 !important;
}

.wf-node-handle-in {
  background: #10b981;
}

.wf-node-handle-out {
  background: #f59e0b;
}

:global(.workflow-editor.dark) .wf-node,
:global(.dark) .wf-node {
  background: #131d2b;
  border-color: #3f7ee8;
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.3);
}

:global(.workflow-editor.dark) .wf-node.selected,
:global(.dark) .wf-node.selected {
  border-color: #6f9df5;
  box-shadow: 0 0 0 4px rgba(96, 165, 250, 0.18), 0 14px 30px rgba(0, 0, 0, 0.34);
}

:global(.workflow-editor.dark) .wf-node-header,
:global(.dark) .wf-node-header {
  border-bottom-color: rgba(148, 163, 184, 0.22);
  background: #1a2637;
}

:global(.workflow-editor.dark) .wf-node-icon,
:global(.dark) .wf-node-icon {
  background: rgba(59, 130, 246, 0.16);
  color: #8bb8ff;
}

:global(.workflow-editor.dark) .wf-node-title,
:global(.dark) .wf-node-title {
  color: #f8fafc;
}

:global(.workflow-editor.dark) .wf-node-content,
:global(.workflow-editor.dark) .wf-node-props-count,
:global(.dark) .wf-node-content,
:global(.dark) .wf-node-props-count {
  color: #d6deea;
}

:global(.workflow-editor.dark) .wf-node-badge,
:global(.dark) .wf-node-badge {
  background: rgba(148, 163, 184, 0.16);
  color: #d8e2f0;
}

:global(.workflow-editor.dark) .wf-node-handle,
:global(.dark) .wf-node-handle {
  border-color: #131d2b !important;
}
</style>
