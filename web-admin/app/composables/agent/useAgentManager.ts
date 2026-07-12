import type {
  Agent,
  AgentListResponse,
  AgentDetailResponse,
  AgentAccessGrant,
  AgentAccessSubjectType,
  AgentEffectivePermissions,
  AgentGrant,
  AgentGrantableCapability,
  CreateAgentRequest,
  UpdateAgentRequest,
} from "~/types/agent";
import { useApiClient } from "~/composables/api/index";
import { useEnvStore } from "~/stores/envStore";

export interface AgentConfig {
  id: string;
  name: string;
  description: string;
  avatar?: string;
  provider?: string;
  model: string;
  systemPrompt: string;
  temperature: number;
  maxTokens: number;
  topP: number;
  frequencyPenalty: number;
  presencePenalty: number;
  isActive: boolean;
  useSystemModelConfig?: boolean;
  capabilities: Array<{
    name: string;
    description: string;
    enabled: boolean;
  }>;
}

export const useAgentManager = () => {
  const agents = ref<Agent[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const envStore = useEnvStore();
  const ENV = computed(() => envStore.currentEnv || "dev");
  const fetchAgentsInFlight = useState<Promise<void> | null>(
    "px-agent-manager-fetch-agents-inflight",
    () => null
  );

  // 使用封装的 API 客户端
  const { get, post, patch, put, delete: del } = useApiClient();

  // 获取 Agent 列表
  const fetchAgents = async () => {
    if (fetchAgentsInFlight.value) {
      return fetchAgentsInFlight.value;
    }
    loading.value = true;
    error.value = null;

    const run = async () => {
      const response = await get<AgentListResponse>("/admin/agents", {
        params: {
          env: ENV.value,
        },
      });

      if (response.code === 200) {
        agents.value = response.data.items;
      } else {
        throw new Error(response.message || "获取 Agent 列表失败");
      }
    };

    const inflight = run();
    fetchAgentsInFlight.value = inflight;
    try {
      await inflight;
    } catch (e: any) {
      error.value = e.message || "网络请求失败";
      console.error("获取 Agent 列表失败:", e);
      throw e;
    } finally {
      loading.value = false;
      if (fetchAgentsInFlight.value === inflight) {
        fetchAgentsInFlight.value = null;
      }
    }
  };

  // 获取单个 Agent 详情
  const fetchAgentDetail = async (agentUUID: string) => {
    try {
      const response = await get<AgentDetailResponse>(
        `/admin/agents/${agentUUID}`,
        {
          params: {
            env: ENV.value,
          },
        }
      );

      if (response.code === 200) {
        return response.data;
      } else {
        throw new Error(response.message || "获取 Agent 详情失败");
      }
    } catch (e: any) {
      console.error("获取 Agent 详情失败:", e);
      throw e;
    }
  };

  // 创建 Agent
  const createAgent = async (agentData: CreateAgentRequest) => {
    try {
      const response = await post<AgentDetailResponse>(
        "/admin/agents",
        { env: ENV.value, ...agentData }
      );

      if (response.code === 200) {
        // 重新获取列表
        await fetchAgents();
        return response.data;
      } else {
        throw new Error(response.message || "创建 Agent 失败");
      }
    } catch (e: any) {
      console.error("创建 Agent 失败:", e);
      throw e;
    }
  };

  // 更新 Agent
  const updateAgent = async (
    agentUUID: string,
    agentData: UpdateAgentRequest
  ) => {
    try {
      // 后端是 PATCH /admin/agents/:uuid?env=xxx（PUT 会 404）
      const payload: any = {};
      if (typeof agentData.name === "string") payload.name = agentData.name;
      if (typeof agentData.description === "string")
        payload.description = agentData.description;
      if (typeof agentData.status === "string") payload.status = agentData.status;
      if (typeof agentData.typeId === "string") payload.typeId = agentData.typeId;
      if (typeof agentData.scene === "string") payload.scene = agentData.scene;
      if (typeof agentData.promptSeed === "string") payload.promptSeed = agentData.promptSeed;
      if (typeof agentData.persona === "string") payload.persona = agentData.persona;
      if (Array.isArray(agentData.skillIds)) payload.skillIds = agentData.skillIds;
      if (Array.isArray(agentData.knowledgeBaseIds))
        payload.knowledgeBaseIds = agentData.knowledgeBaseIds;
      if (agentData.meta && typeof agentData.meta === "object")
        payload.meta = agentData.meta;
      const response = await patch<AgentDetailResponse>(
        `/admin/agents/${agentUUID}`,
        payload,
        { params: { env: ENV.value } }
      );

      if (response.code === 200) {
        // 重新获取列表
        await fetchAgents();
        return response.data;
      } else {
        throw new Error(response.message || "更新 Agent 失败");
      }
    } catch (e: any) {
      console.error("更新 Agent 失败:", e);
      throw e;
    }
  };

  // 删除 Agent
  const deleteAgent = async (agentUUID: string) => {
    try {
      const response = await del(`/admin/agents/${agentUUID}`, {
        params: { env: ENV.value },
      });

      // 重新获取列表
      await fetchAgents();
      return response;
    } catch (e: any) {
      console.error("删除 Agent 失败:", e);
      throw e;
    }
  };

  const fetchGrantableCapabilities = async () => {
    const response = await get<{
      code: number;
      message: string;
      data: { items: AgentGrantableCapability[] };
    }>("/admin/agents/grantable-capabilities", {
      params: { env: ENV.value },
    });
    if (response.code !== 200) {
      throw new Error(response.message || "agent grants catalog failed");
    }
    return response.data.items || [];
  };

  const fetchAgentGrants = async (agentUUID: string) => {
    const response = await get<{
      code: number;
      message: string;
      data: { items: AgentGrant[] };
    }>(`/admin/agents/${agentUUID}/grants`, {
      params: { env: ENV.value },
    });
    if (response.code !== 200) {
      throw new Error(response.message || "agent grants failed");
    }
    return response.data.items || [];
  };

  const updateAgentGrants = async (
    agentUUID: string,
    grants: Array<{ capability_uuid: string; permission_code: string; enabled: boolean }>
  ) => {
    const response = await patch<{
      code: number;
      message: string;
      data: { items: AgentGrant[] };
    }>(
      `/admin/agents/${agentUUID}/grants`,
      { grants },
      { params: { env: ENV.value } }
    );
    if (response.code !== 200) {
      throw new Error(response.message || "agent grants update failed");
    }
    return response.data.items || [];
  };

  const fetchMyEffectivePermissions = async (agentUUID: string) => {
    const response = await get<{
      code: number;
      message: string;
      data: AgentEffectivePermissions;
    }>(`/admin/agents/${agentUUID}/my-effective-permissions`, {
      params: { env: ENV.value },
    });
    if (response.code !== 200) {
      throw new Error(response.message || "agent effective permissions failed");
    }
    return response.data;
  };

  const fetchEffectivePermissions = async (agentUUID: string, memberUUID: string) => {
    const response = await get<{
      code: number;
      message: string;
      data: AgentEffectivePermissions;
    }>(`/admin/agents/${agentUUID}/effective-permissions`, {
      params: { env: ENV.value, member_uuid: memberUUID },
    });
    if (response.code !== 200) {
      throw new Error(response.message || "agent effective permissions failed");
    }
    return response.data;
  };

  const fetchAgentAccessGrants = async (
    agentUUID: string,
    subjectType?: AgentAccessSubjectType
  ) => {
    const response = await get<{
      code: number;
      message: string;
      data: { items: AgentAccessGrant[] };
    }>(`/admin/agents/${agentUUID}/access-grants`, {
      params: { env: ENV.value, ...(subjectType ? { subject_type: subjectType } : {}) },
    });
    if (response.code !== 200) {
      throw new Error(response.message || "agent access grants failed");
    }
    return response.data.items || [];
  };

  const updateAgentAccessGrants = async (
    agentUUID: string,
    grants: Array<{ subject_type: AgentAccessSubjectType; subject_uuid: string; enabled: boolean }>
  ) => {
    const response = await patch<{
      code: number;
      message: string;
      data: { items: AgentAccessGrant[] };
    }>(
      `/admin/agents/${agentUUID}/access-grants`,
      { grants },
      { params: { env: ENV.value } }
    );
    if (response.code !== 200) {
      throw new Error(response.message || "agent access grants update failed");
    }
    return response.data.items || [];
  };

  return {
    agents: readonly(agents),
    loading: readonly(loading),
    error: readonly(error),
    fetchAgents,
    fetchAgentDetail,
    createAgent,
    updateAgent,
    deleteAgent,
    fetchGrantableCapabilities,
    fetchAgentGrants,
    updateAgentGrants,
    fetchMyEffectivePermissions,
    fetchEffectivePermissions,
    fetchAgentAccessGrants,
    updateAgentAccessGrants,
  };
};
