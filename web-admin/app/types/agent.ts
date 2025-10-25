// Agent 相关类型定义
export interface Agent {
  id: number;
  createdAt: string;
  updatedAt: string;
  DeletedAt: string | null;
  key: string;
  name: string;
  description: string;
  source: string;
  scope: string;
  visibility: string;
  status: "active" | "inactive";
  defaultPersonaId?: number;
  blueprintRefs?: Array<{
    id: string;
    entry: string;
    version: string;
  }>;
  intentCardsRef?: Array<{
    name: string;
    hints: string[];
    priority: number;
    threshold: {
      low: number;
      high: number;
    };
  }>;
  toolAllowlist?: string[];
  kbStrategy: string;
  meta: {
    builtin?: boolean;
    icon?: string;
    protect_from_delete?: boolean;
    tags?: string[];
    [key: string]: any;
  };
}

export interface AgentListResponse {
  code: number;
  message: string;
  data: {
    items: Agent[];
  };
  timestamp: number;
}

export interface AgentDetailResponse {
  code: number;
  message: string;
  data: Agent;
  timestamp: number;
}

// 用于创建/更新 Agent 的类型
export interface CreateAgentRequest {
  key: string;
  name: string;
  description: string;
  status: "active" | "inactive";
  meta?: Record<string, any>;
}

export interface UpdateAgentRequest extends Partial<CreateAgentRequest> {
  id: number;
}
