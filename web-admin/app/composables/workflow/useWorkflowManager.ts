import { ref, reactive, computed } from 'vue';
import type { Node, Edge } from '@vue-flow/core';
import type { KindSpec, PaletteItem, Workflow } from '~/types/workflow';
import { useWorkflowTranslator } from './useWorkflowTranslator';
import { useWorkflowService } from '~/composables/api/services/workflowService';

/**
 * 工作流管理器 - 管理工作流的状态和操作
 */
export function useWorkflowManager() {
  const { makeNodeFromPalette, makeEdgeFromWorkflowEdge, makeWorkflowFromVueFlow } = useWorkflowTranslator();
  const workflowService = useWorkflowService();

  // 状态
  const kinds = ref<Record<string, KindSpec>>({});
  const palette = ref<PaletteItem[]>([]);
  const currentWorkflow = ref<Workflow | null>(null);
  const isLoading = ref(false);
  const error = ref<string | null>(null);

  // 加载Kind规格
  async function loadKinds() {
    try {
      isLoading.value = true;
      const data = await workflowService.getKinds();
      
      // 转换为Record格式方便查找
      const kindsMap: Record<string, KindSpec> = {};
      data.kinds.forEach((kind: KindSpec) => {
        kindsMap[kind.kind] = kind;
      });
      
      kinds.value = kindsMap;
    } catch (err) {
      error.value = '加载节点类型失败';
      console.error(err);
    } finally {
      isLoading.value = false;
    }
  }

  // 加载Palette清单
  async function loadPalette() {
    try {
      isLoading.value = true;
      const data = await workflowService.getPalette();
      palette.value = data.palette;
    } catch (err) {
      error.value = '加载节点模板失败';
      console.error(err);
    } finally {
      isLoading.value = false;
    }
  }

  // 加载工作流
  async function loadWorkflow(id: string) {
    try {
      isLoading.value = true;
      const workflow = await workflowService.getWorkflow(id);
      currentWorkflow.value = workflow;
      
      // 这里需要在实际的Vue Flow组件中处理节点和边的加载
      // 因为useVueFlow只能在Vue Flow组件内部使用
    } catch (err) {
      error.value = '加载工作流失败';
      console.error(err);
    } finally {
      isLoading.value = false;
    }
  }

  // 从Palette添加节点到画布
  function addNodeFromPalette(paletteId: string, position: { x: number, y: number }) {
    const paletteItem = palette.value.find(p => p.id === paletteId);
    if (!paletteItem) {
      error.value = `未找到模板: ${paletteId}`;
      return null;
    }
    
    try {
      const node = makeNodeFromPalette(paletteItem, kinds.value, position);
      return node;
    } catch (err) {
      error.value = `创建节点失败: ${err}`;
      console.error(err);
      return null;
    }
  }

  // 保存当前工作流
  async function saveWorkflow(nodes: Node[], edges: Edge[]) {
    if (!currentWorkflow.value) return;
    
    try {
      isLoading.value = true;
      
      const { nodes: wfNodes, edges: wfEdges } = makeWorkflowFromVueFlow(nodes, edges);
      
      const updatedWorkflow = await workflowService.updateWorkflow(currentWorkflow.value.id, {
        name: currentWorkflow.value.name,
        description: currentWorkflow.value.description,
        nodes: wfNodes,
        edges: wfEdges
      });
      
      currentWorkflow.value = updatedWorkflow.data;
    } catch (err) {
      error.value = '保存工作流失败';
      console.error(err);
    } finally {
      isLoading.value = false;
    }
  }

  // 创建新工作流
  async function createNewWorkflow(name: string, description: string = '') {
    try {
      isLoading.value = true;
      const result = await workflowService.createWorkflow({ name, description });
      currentWorkflow.value = result.data;
      return result.data;
    } catch (err) {
      error.value = '创建工作流失败';
      console.error(err);
      return null;
    } finally {
      isLoading.value = false;
    }
  }

  // 获取工作流列表
  async function getWorkflowList() {
    try {
      isLoading.value = true;
      const result = await workflowService.getWorkflowList();
      return result.workflows;
    } catch (err) {
      error.value = '获取工作流列表失败';
      console.error(err);
      return [];
    } finally {
      isLoading.value = false;
    }
  }

  return {
    kinds,
    palette,
    currentWorkflow,
    isLoading,
    error,
    loadKinds,
    loadPalette,
    loadWorkflow,
    addNodeFromPalette,
    saveWorkflow,
    createNewWorkflow,
    getWorkflowList,
  };
}