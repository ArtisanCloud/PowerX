import type {
  Agent,
  AgentListResponse,
  AgentDetailResponse,
  CreateAgentRequest,
  UpdateAgentRequest,
} from "~/types/agent";
import { useApiClient } from "~/composables/api/index";

export interface AgentConfig {
  id: string;
  name: string;
  description: string;
  avatar?: string;
  model: string;
  systemPrompt: string;
  temperature: number;
  maxTokens: number;
  topP: number;
  frequencyPenalty: number;
  presencePenalty: number;
  isActive: boolean;
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

  // 使用封装的 API 客户端
  const { get, post, put, delete: del } = useApiClient();

  // 获取 Agent 列表
  const fetchAgents = async () => {
    loading.value = true;
    error.value = null;

    try {
      const response = await get<AgentListResponse>("/admin/agents", {
        params: {
          env: "dev",
          status: "active",
        },
      });

      if (response.code === 200) {
        agents.value = response.data.items;
      } else {
        throw new Error(response.message || "获取 Agent 列表失败");
      }
    } catch (e: any) {
      error.value = e.message || "网络请求失败";
      console.error("获取 Agent 列表失败:", e);
      throw e;
    } finally {
      loading.value = false;
    }
  };

  // 获取单个 Agent 详情
  const fetchAgentDetail = async (agentId: number) => {
    try {
      const response = await get<AgentDetailResponse>(
        `/admin/agents/${agentId}`,
        {
          params: {
            modality: "llm",
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
        agentData
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
    agentId: number,
    agentData: UpdateAgentRequest
  ) => {
    try {
      const response = await put<AgentDetailResponse>(
        `/admin/agents/${agentId}`,
        agentData
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
  const deleteAgent = async (agentId: number) => {
    try {
      const response = await del(`/admin/agents/${agentId}`);

      // 重新获取列表
      await fetchAgents();
      return response;
    } catch (e: any) {
      console.error("删除 Agent 失败:", e);
      throw e;
    }
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
  };
};
