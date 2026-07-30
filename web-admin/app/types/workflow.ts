import type { WorkflowDefinition, WorkflowNodeCatalogItem } from "~/composables/api/services/workflowService";

export interface KindSpec {
  kind: string;
  version: string;
  label: string;
  ports: { inputs: { name: string }[]; outputs: { name: string; label?: string }[] };
  defaultProps: Record<string, any>;
  schema: Record<string, any>;
  ui: {
    shape: "card" | "diamond" | "pill" | "oval" | "minimal";
    colorToken?: string;
    icon?: string;
    size?: { w: number; h: number };
    badges?: string[];
    handles?: { left?: string[]; right?: string[]; top?: string[]; bottom?: string[] };
  };
}

export interface PaletteItem {
  id: string;
  kind: string;
  label: string;
  icon?: string;
  defaultProps?: Record<string, any>;
  uiOverrides?: Partial<KindSpec["ui"]> & { previewTpl?: string; component?: string };
  catalogItem?: WorkflowNodeCatalogItem;
}

export interface WfNode {
  id: string;
  kind: string;
  paletteId: string;
  label: string;
  props: Record<string, any>;
  ui: {
    shape: string;
    colorToken?: string;
    icon?: string;
    size?: { w: number; h: number };
    badges?: string[];
    previewTpl?: string;
    component?: string;
  };
  position: { x: number; y: number };
}

export interface Workflow {
  id: string;
  uuid: string;
  name: string;
  description?: string;
  nodes: WfNode[];
  edges: Edge[];
  version: string;
  status?: string;
  createdAt: string;
  updatedAt: string;
  raw?: WorkflowDefinition;
}

export interface Edge {
  id: string;
  source: string;
  sourceHandle: string;
  target: string;
  targetHandle: string;
  label?: string;
  type?: string;
}

export interface Port {
  name: string;
  label?: string;
}

export interface NodeData {
  kind: string;
  paletteId: string;
  label: string;
  props: Record<string, any>;
  ui: KindSpec["ui"] & { previewTpl?: string; component?: string };
  ports: { inputs: Port[]; outputs: Port[] };
  schema: Record<string, any>;
  runState?: string;
}
