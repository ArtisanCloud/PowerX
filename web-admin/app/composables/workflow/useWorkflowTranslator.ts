import type { Node, Edge as VueFlowEdge } from '@vue-flow/core';
import type { KindSpec, PaletteItem, Edge } from '~/types/workflow';

/**
 * 工作流转译器 - 将后端数据转换为Vue Flow可用的节点和边
 */
export function useWorkflowTranslator() {
  /**
   * 从Palette创建Vue Flow节点
   */
  function makeNodeFromPalette(
    p: PaletteItem,
    kindMap: Record<string, KindSpec>,
    position = { x: 120, y: 100 }
  ): Node {
    const k = kindMap[p.kind];
    if (!k) throw new Error(`未知节点类型: ${p.kind}`);

    const mergedUI = { ...k.ui, ...(p.uiOverrides || {}) };
    const props = { ...k.defaultProps, ...(p.defaultProps || {}) };

    // 生成唯一ID
    const id = `${p.id}-${crypto.randomUUID()}`;

    const data = {
      kind: k.kind,
      paletteId: p.id,
      label: p.label ?? k.label,
      props,
      ui: mergedUI,
      ports: k.ports,
      schema: k.schema, // 供属性面板渲染与即时校验
    };

    // Vue Flow 的 node.type 建议统一用 "wf-generic"（通用渲染器）
    // 若 mergedUI.component 存在，可在渲染器内走注册表加载定制组件
    const node: Node = {
      id,
      type: 'wf-generic',
      position,
      data,
      width: mergedUI.size?.w,
      height: mergedUI.size?.h
    };
    return node;
  }

  /**
   * 将工作流边转换为Vue Flow边
   */
  function makeEdgeFromWorkflowEdge(edge: Edge): VueFlowEdge {
    return {
      id: edge.id,
      source: edge.source,
      sourceHandle: edge.sourceHandle,
      target: edge.target,
      targetHandle: edge.targetHandle,
      label: edge.label,
      type: edge.type || 'smoothstep',
      animated: true,
    };
  }

  /**
   * 从Vue Flow节点和边转换回可保存的工作流数据
   */
  function makeWorkflowFromVueFlow(nodes: Node[], edges: VueFlowEdge[]) {
    const wfNodes = nodes.map(node => ({
      id: node.id,
      kind: node.data.kind,
      paletteId: node.data.paletteId,
      label: node.data.label,
      props: node.data.props,
      ui: node.data.ui,
      position: node.position,
    }));

    const wfEdges = edges.map(edge => ({
      id: edge.id,
      source: edge.source,
      sourceHandle: edge.sourceHandle || '',
      target: edge.target,
      targetHandle: edge.targetHandle || '',
      label: typeof edge.label === 'string' ? edge.label : undefined,
      type: edge.type,
    }));

    return {
      nodes: wfNodes,
      edges: wfEdges,
    };
  }

  return {
    makeNodeFromPalette,
    makeEdgeFromWorkflowEdge,
    makeWorkflowFromVueFlow,
  };
}