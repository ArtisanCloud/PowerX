import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";
import { useOneShotAlert } from "../../useOneShotAlert";

// 部门接口定义
export interface Department {
  id: number;
  name: string;
  sort?: number;
  leader_member_id?: number | null;
  parent_id?: number | null;
  key?: string;
  status?: number;
  meta?: any;
  children?: Department[];
}

// 部门创建参数
export interface DepartmentCreateParams {
  name: string;
  parent_id?: number;
}

// 部门更新参数
export type DepartmentUpdateParams = {
  name?: string;
  key?: string;
  new_parent_id?: number | null; // 为空不移动（不传表示不改）
  sort?: number;
  leader_member_id?: number | null;
  status?: number; // int16 后端能接 number
  meta?: any; // 将在请求时 JSON.stringify
};

/**
 * 从任意响应结构中解析出部门树形结构；取不到就返回 []，保证上层永远拿到数组
 */
function parseDepartmentsFromResponse(resp: any): Department[] {
  const data = resp?.data ?? resp;
  const departments = Array.isArray(data) ? data : [];
  return normalizeDepartmentTree(departments);
}

/**
 * 规范化部门树结构，确保数据格式一致
 */
function normalizeDepartmentTree(departments: any[]): Department[] {
  return departments
    .filter((dept) => dept && typeof dept === "object")
    .map((dept) => {
      const children = Array.isArray(dept.children)
        ? normalizeDepartmentTree(dept.children)
        : undefined;

      const item: Department = {
        id: Number(dept.id),
        name: String(dept.name || ""),
        parent_id:
          typeof dept.parent_id === "number" ? dept.parent_id : undefined,
        children,
        ...dept,
      };

      return item;
    });
}

/**
 * 部门服务 API
 */
export function useDepartmentService() {
  const apiClient = useApiClient();
  const baseUrl = "/admin/iam/departments";

  return {
    /**
     * 获取部门树形结构
     * 统一返回 Department[]
     */
    getDepartmentTree: async (): Promise<Department[]> => {
      try {
        // 先尝试 /tree 端点
        let res;
        try {
          res = await apiClient.get<ApiResponse<Department[]>>(
            `${baseUrl}/tree`
          );
        } catch (treeError: any) {
          console.warn("尝试 /tree 端点失败，尝试使用基础端点:", treeError);
          // 如果 /tree 不存在，尝试使用基础端点
          res = await apiClient.get<ApiResponse<Department[]>>(baseUrl);
        }

        const serverResp = res?.data ?? res;
        // console.info("获取部门数据成功:", serverResp);
        return parseDepartmentsFromResponse(serverResp);
      } catch (error) {
        throw error;
        // return [];
      }
    },

    /**
     * 创建部门
     */
    createDepartment: async (
      data: DepartmentCreateParams
    ): Promise<Department | null> => {
      try {
        const res = await apiClient.post<ApiResponse<Department>>(
          baseUrl,
          data
        );
        const serverResp = res?.data ?? res;
        return serverResp;
      } catch (error) {
        console.error("创建部门失败:", error);
        throw error;
      }
    },

    /**
     * 更新部门
     */
    updateDepartment: async (
      id: number,
      data: DepartmentUpdateParams
    ): Promise<Department | null> => {
      try {
        const res = await apiClient.patch<ApiResponse<Department>>(
          `${baseUrl}/${id}`,
          data
        );
        const serverResp = res?.data ?? res;
        // 成功提醒
        return serverResp;
      } catch (error) {
        console.error("更新部门失败:", error);
        // 失败提醒
        throw error;
      }
    },

    /**
     * 删除部门
     */
    deleteDepartment: async (id: number): Promise<boolean> => {
      try {
        await apiClient.delete<ApiResponse<null>>(`${baseUrl}/${id}`);
        return true;
      } catch (error) {
        throw error;
      }
    },

    /**
     * 获取指定部门信息
     */
    getDepartment: async (id: number): Promise<Department | null> => {
      try {
        const res = await apiClient.get<ApiResponse<Department>>(
          `${baseUrl}/${id}`
        );
        const serverResp = res?.data ?? res;
        return serverResp.data;
      } catch (error) {
        throw error;
      }
    },
  };
}
