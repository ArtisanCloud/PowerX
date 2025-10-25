// Kind 规格（服务端下发）
export interface KindSpec {
  kind: string;            // 如 "llm"
  version: string;
  label: string;
  ports: { inputs: {name:string}[]; outputs: {name:string; label?:string}[] };
  defaultProps: Record<string, any>;
  schema: Record<string, any>;  // JSON Schema
  ui: {
    shape: "card" | "diamond" | "pill" | "minimal";
    colorToken?: string;        // primary/warning/info/...
    icon?: string;
    size?: { w: number; h: number };
    badges?: string[];
    handles?: { left?: string[]; right?: string[]; top?: string[]; bottom?: string[] };
  };
}

// Palette 模板（服务端下发）
export interface PaletteItem {
  id: string;              // 如 "http.post.json"
  kind: string;            // 指向 Kind
  label: string;
  icon?: string;
  defaultProps?: Record<string, any>;
  uiOverrides?: Partial<KindSpec["ui"]> & { previewTpl?: string; component?: string };
}

// 画布实例（保存）
export interface WfNode {
  id: string;
  kind: string;            // 决定执行行为
  paletteId: string;       // 来源模板
  label: string;
  props: Record<string, any>;
  ui: {
    shape: string; 
    colorToken?: string; 
    icon?: string; 
    size?: {w:number;h:number};
    badges?: string[]; 
    previewTpl?: string; 
    component?: string;
  };
  position: { x: number; y: number };
}

// 工作流定义
export interface Workflow {
  id: string;
  name: string;
  description?: string;
  nodes: WfNode[];
  edges: Edge[];
  version: string;
  createdAt: string;
  updatedAt: string;
}

// 边定义
export interface Edge {
  id: string;
  source: string;
  sourceHandle: string;
  target: string;
  targetHandle: string;
  label?: string;
  type?: string;
}

// 端口定义
export interface Port {
  name: string;
  label?: string;
}

// 节点数据
export interface NodeData {
  kind: string;
  paletteId: string;
  label: string;
  props: Record<string, any>;
  ui: KindSpec['ui'] & { previewTpl?: string; component?: string };
  ports: { inputs: Port[]; outputs: Port[] };
  schema: Record<string, any>;
}