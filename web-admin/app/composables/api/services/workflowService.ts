import { useApiClient } from "../index";
import type {
  ApiResponse,
  PaginatedResponse,
  PaginationParams,
} from "../types/types";
import type {
  KindSpec,
  PaletteItem,
  Workflow,
  WfNode,
  Edge,
} from "~/types/workflow";

/**
 * 工作流相关类型定义
 */
export interface WorkflowCreateParams {
  name: string;
  description?: string;
  category?: string;
  tags?: string[];
  isPublic?: boolean;
}

export interface WorkflowUpdateParams {
  name?: string;
  description?: string;
  category?: string;
  tags?: string[];
  isPublic?: boolean;
  nodes?: WfNode[];
  edges?: Edge[];
  status?: "draft" | "published" | "archived";
}

export interface WorkflowFilters extends PaginationParams {
  category?: string;
  status?: "draft" | "published" | "archived";
  tags?: string[];
  search?: string;
  authorId?: string;
  isPublic?: boolean;
}

export interface WorkflowExecutionParams {
  input?: Record<string, any>;
  context?: Record<string, any>;
  variables?: Record<string, any>;
}

export interface WorkflowExecution {
  id: string;
  workflowId: string;
  status: "running" | "completed" | "failed" | "cancelled";
  input: Record<string, any>;
  output?: Record<string, any>;
  error?: string;
  startedAt: string;
  completedAt?: string;
  duration?: number;
  nodeExecutions: NodeExecution[];
}

export interface NodeExecution {
  id: string;
  nodeId: string;
  status: "pending" | "running" | "completed" | "failed" | "skipped";
  input?: Record<string, any>;
  output?: Record<string, any>;
  error?: string;
  startedAt?: string;
  completedAt?: string;
  duration?: number;
}

export interface WorkflowTemplate {
  id: string;
  name: string;
  description: string;
  category: string;
  tags: string[];
  thumbnail?: string;
  workflow: Omit<Workflow, "id" | "createdAt" | "updatedAt">;
  usageCount: number;
  rating: number;
  author: {
    id: string;
    name: string;
    avatar?: string;
  };
  createdAt: string;
  updatedAt: string;
}

export interface WorkflowListResponse {
  workflows: Array<{
    id: string;
    name: string;
    description?: string;
    category?: string;
    tags: string[];
    status: "draft" | "published" | "archived";
    isPublic: boolean;
    nodeCount: number;
    executionCount: number;
    lastExecutedAt?: string;
    createdAt: string;
    updatedAt: string;
  }>;
}

export interface KindsResponse {
  apiVersion: string;
  kinds: KindSpec[];
}

export interface PaletteResponse {
  apiVersion: string;
  palette: PaletteItem[];
}

export interface WorkflowValidationResult {
  valid: boolean;
  errors: Array<{
    type: "error" | "warning";
    message: string;
    nodeId?: string;
    edgeId?: string;
  }>;
}

export interface WorkflowStats {
  totalWorkflows: number;
  publishedWorkflows: number;
  totalExecutions: number;
  successRate: number;
  avgExecutionTime: number;
  popularCategories: Array<{
    category: string;
    count: number;
  }>;
}

/**
 * 工作流服务 API
 */
export const useWorkflowService = () => {
  const apiClient = useApiClient();
  const baseUrl = "/workflow";

  return {
    /**
     * 获取节点类型规格
     */
    getKinds: (): Promise<KindsResponse> => {
      // 模拟数据 - 实际应该调用 API
      const mockKinds: KindsResponse = {
        apiVersion: "corex.wf/v1",
        kinds: [
          {
            kind: "llm",
            version: "1.0.0",
            label: "大模型",
            ports: {
              inputs: [{ name: "in" }],
              outputs: [{ name: "out" }, { name: "error" }],
            },
            defaultProps: {
              model: "gpt-4o",
              prompt: "",
              temperature: 0.3,
            },
            schema: {
              type: "object",
              required: ["model", "prompt"],
              properties: {
                model: { type: "string" },
                prompt: { type: "string", minLength: 0 },
                temperature: { type: "number", minimum: 0, maximum: 1 },
              },
            },
            ui: {
              shape: "card",
              colorToken: "primary",
              icon: "i-heroicons-cpu-chip",
              size: { w: 220, h: 96 },
              badges: ["AI"],
              handles: {
                left: ["in"],
                right: ["out", "error"],
              },
            },
          },
          {
            kind: "http",
            version: "1.0.0",
            label: "HTTP 请求",
            ports: {
              inputs: [{ name: "in" }],
              outputs: [{ name: "ok" }, { name: "fail" }],
            },
            defaultProps: {
              method: "GET",
              url: "",
              headers: {},
              body: {},
            },
            schema: {
              type: "object",
              required: ["method", "url"],
              properties: {
                method: {
                  type: "string",
                  enum: ["GET", "POST", "PUT", "PATCH", "DELETE"],
                },
                url: { type: "string" },
                headers: {
                  type: "object",
                  additionalProperties: { type: "string" },
                },
                body: { type: "object" },
              },
            },
            ui: {
              shape: "card",
              colorToken: "info",
              icon: "i-heroicons-globe-alt",
              size: { w: 260, h: 110 },
              badges: ["IO"],
            },
          },
          {
            kind: "selector",
            version: "1.0.0",
            label: "条件",
            ports: {
              inputs: [{ name: "in" }],
              outputs: [{ name: "true" }, { name: "false" }],
            },
            defaultProps: {
              expr: "vars.score >= 80",
            },
            schema: {
              type: "object",
              required: ["expr"],
              properties: {
                expr: { type: "string" },
              },
            },
            ui: {
              shape: "diamond",
              colorToken: "warning",
              icon: "i-heroicons-arrow-path",
              size: { w: 160, h: 120 },
            },
          },
        ],
      };

      return Promise.resolve(mockKinds);
      // 实际实现应该是：
      // return apiClient.get<KindsResponse>(`${baseUrl}/kinds`);
    },

    /**
     * 获取节点模板清单
     */
    getPalette: (): Promise<PaletteResponse> => {
      // 模拟数据 - 实际应该调用 API
      const mockPalette: PaletteResponse = {
        apiVersion: "corex.wf/v1",
        palette: [
          {
            id: "llm.basic",
            kind: "llm",
            label: "LLM（通用）",
            icon: "i-heroicons-cpu-chip",
            defaultProps: {
              model: "gpt-4o",
              temperature: 0.2,
              prompt: "请根据输入生成摘要",
            },
            uiOverrides: {
              colorToken: "primary",
              previewTpl: "{{props.model}} · T={{props.temperature}}",
            },
          },
          {
            id: "llm.summarize",
            kind: "llm",
            label: "LLM（摘要）",
            icon: "i-heroicons-document-text",
            defaultProps: {
              model: "gpt-4o",
              temperature: 0.1,
              prompt: "请对以下内容进行摘要，提取关键信息：",
            },
            uiOverrides: {
              colorToken: "success",
              previewTpl: "摘要生成器",
            },
          },
          {
            id: "http.get",
            kind: "http",
            label: "HTTP GET",
            icon: "i-heroicons-arrow-down-on-square",
            defaultProps: {
              method: "GET",
              url: "https://api.example.com/v1/items",
            },
            uiOverrides: {
              colorToken: "info",
              previewTpl: "{{props.method}} {{props.url}}",
            },
          },
          {
            id: "http.post.json",
            kind: "http",
            label: "HTTP POST(JSON)",
            icon: "i-heroicons-arrow-up-on-square",
            defaultProps: {
              method: "POST",
              url: "https://api.example.com/v1/items",
              headers: { "Content-Type": "application/json" },
              body: { name: "" },
            },
            uiOverrides: {
              colorToken: "success",
            },
          },
          {
            id: "selector.score",
            kind: "selector",
            label: "高意向？(score>=80)",
            icon: "i-heroicons-adjustments-horizontal",
            defaultProps: {
              expr: "vars.score >= 80",
            },
          },
        ],
      };

      return Promise.resolve(mockPalette);
      // 实际实现应该是：
      // return apiClient.get<PaletteResponse>(`${baseUrl}/palette`);
    },

    /**
     * 获取工作流列表
     */
    getWorkflows: (filters?: WorkflowFilters) => {
      return apiClient.get<
        ApiResponse<PaginatedResponse<WorkflowListResponse>>
      >(baseUrl, { params: filters });
    },

    /**
     * 获取指定工作流
     */
    getWorkflow: (id: string) => {
      return apiClient.get<ApiResponse<Workflow>>(`${baseUrl}/${id}`);
    },

    /**
     * 创建工作流
     */
    createWorkflow: (data: WorkflowCreateParams) => {
      return apiClient.post<ApiResponse<Workflow>>(baseUrl, data);
    },

    /**
     * 更新工作流
     */
    updateWorkflow: (id: string, data: WorkflowUpdateParams) => {
      return apiClient.put<ApiResponse<Workflow>>(`${baseUrl}/${id}`, data);
    },

    /**
     * 删除工作流
     */
    deleteWorkflow: (id: string) => {
      return apiClient.delete<ApiResponse<null>>(`${baseUrl}/${id}`);
    },

    /**
     * 复制工作流
     */
    duplicateWorkflow: (id: string, name?: string) => {
      return apiClient.post<ApiResponse<Workflow>>(
        `${baseUrl}/${id}/duplicate`,
        {
          name,
        }
      );
    },

    /**
     * 发布工作流
     */
    publishWorkflow: (id: string) => {
      return apiClient.post<ApiResponse<Workflow>>(`${baseUrl}/${id}/publish`);
    },

    /**
     * 归档工作流
     */
    archiveWorkflow: (id: string) => {
      return apiClient.post<ApiResponse<Workflow>>(`${baseUrl}/${id}/archive`);
    },

    /**
     * 验证工作流
     */
    validateWorkflow: (workflow: Partial<Workflow>) => {
      return apiClient.post<ApiResponse<WorkflowValidationResult>>(
        `${baseUrl}/validate`,
        workflow
      );
    },

    /**
     * 执行工作流
     */
    executeWorkflow: (id: string, params?: WorkflowExecutionParams) => {
      return apiClient.post<ApiResponse<WorkflowExecution>>(
        `${baseUrl}/${id}/execute`,
        params
      );
    },

    /**
     * 停止工作流执行
     */
    stopExecution: (executionId: string) => {
      return apiClient.post<ApiResponse<null>>(
        `${baseUrl}/executions/${executionId}/stop`
      );
    },

    /**
     * 获取工作流执行历史
     */
    getExecutions: (workflowId: string, filters?: PaginationParams) => {
      return apiClient.get<ApiResponse<PaginatedResponse<WorkflowExecution>>>(
        `${baseUrl}/${workflowId}/executions`,
        { params: filters }
      );
    },

    /**
     * 获取执行详情
     */
    getExecution: (executionId: string) => {
      return apiClient.get<ApiResponse<WorkflowExecution>>(
        `${baseUrl}/executions/${executionId}`
      );
    },

    /**
     * 获取工作流模板
     */
    getTemplates: (filters?: WorkflowFilters) => {
      return apiClient.get<ApiResponse<PaginatedResponse<WorkflowTemplate>>>(
        `${baseUrl}/templates`,
        { params: filters }
      );
    },

    /**
     * 从模板创建工作流
     */
    createFromTemplate: (templateId: string, name: string) => {
      return apiClient.post<ApiResponse<Workflow>>(
        `${baseUrl}/templates/${templateId}/create`,
        { name }
      );
    },

    /**
     * 导出工作流
     */
    exportWorkflow: (id: string, format: "json" | "yaml" = "json") => {
      return apiClient.get<Blob>(`${baseUrl}/${id}/export`, {
        params: { format },
      });
    },

    /**
     * 导入工作流
     */
    importWorkflow: (file: File) => {
      const formData = new FormData();
      formData.append("file", file);

      return apiClient.upload<ApiResponse<Workflow>>(
        `${baseUrl}/import`,
        formData
      );
    },

    /**
     * 获取工作流统计
     */
    getStats: () => {
      return apiClient.get<ApiResponse<WorkflowStats>>(`${baseUrl}/stats`);
    },

    /**
     * 搜索工作流
     */
    searchWorkflows: (
      query: string,
      filters?: Omit<WorkflowFilters, "search">
    ) => {
      return apiClient.get<
        ApiResponse<PaginatedResponse<WorkflowListResponse>>
      >(`${baseUrl}/search`, { params: { search: query, ...filters } });
    },

    /**
     * 获取工作流分类
     */
    getCategories: () => {
      return apiClient.get<ApiResponse<Array<{ name: string; count: number }>>>(
        `${baseUrl}/categories`
      );
    },

    /**
     * 获取热门标签
     */
    getPopularTags: (limit?: number) => {
      return apiClient.get<ApiResponse<Array<{ tag: string; count: number }>>>(
        `${baseUrl}/tags/popular`,
        { params: { limit } }
      );
    },
  };
};
