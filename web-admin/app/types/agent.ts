// Agent 相关类型定义
export interface Agent {
  id: number;
  uuid: string;
  createdAt: string;
  updatedAt: string;
  DeletedAt: string | null;
  key: string;
  name: string;
  description: string;
  typeId?: string;
  scene?: string;
  promptSeed?: string;
  persona?: string;
  source: string;
  ownerPluginId?: string;
  ownerTenantUuid?: string;
  managedByPlugin?: boolean;
  scope: string;
  visibility: string;
  status: string;
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
  env?: string;
  key: string;
  name: string;
  description: string;
  typeId?: string;
  scene?: string;
  promptSeed?: string;
  persona?: string;
  skillIds?: string[];
  knowledgeBaseIds?: string[];
  status: "draft" | "active" | "disabled";
  meta?: Record<string, any>;
}

export interface UpdateAgentRequest extends Partial<CreateAgentRequest> {
  id?: number;
  uuid?: string;
}

export interface AgentGrantableCapability {
  capability_uuid: string;
  capability_id: string;
  plugin_id: string;
  plugin_uuid?: string;
  display_name: string;
  description?: string;
  permission_code: string;
  risk_level: string;
  agent_usable: boolean;
  tenant_enabled: boolean;
  status: string;
}

export interface AgentGrant {
  uuid: string;
  agent_uuid: string;
  capability_uuid: string;
  plugin_uuid?: string;
  capability_id: string;
  plugin_id?: string;
  permission_code: string;
  risk_level: string;
  status: "enabled" | "disabled";
  source: string;
}

export interface AgentEffectivePermissionItem {
  capability_uuid: string;
  capability_id: string;
  plugin_id: string;
  display_name: string;
  permission_code: string;
  risk_level: string;
  user_allowed: boolean;
  agent_allowed: boolean;
  tenant_enabled: boolean;
  policy_allowed: boolean;
  effective_allowed: boolean;
  deny_reason?: string;
}

export interface AgentEffectivePermissions {
  tenant_uuid: string;
  user_uuid: string;
  member_uuid: string;
  agent_uuid: string;
  items: AgentEffectivePermissionItem[];
}
