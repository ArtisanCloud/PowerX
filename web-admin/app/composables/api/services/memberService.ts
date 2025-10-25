// memberService.go  (TypeScript 文件，命名沿用你的示例)
// 与 departmentService.go 保持同样的风格与错误处理

import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

/** 成员（人员）接口定义 */
export interface Member {
  id: number;
  name: string;
  mobile?: string | null;
  email?: string | null;
  title?: string | null; // 职务/岗位
  department_id?: number | null;
  leader_member_id?: number | null;
  sort?: number | null;
  status?: number | null; // int16/number
  meta?: any; // 后端可返回任意扩展信息
  // 允许服务端附带额外字段，不影响前端使用
  [key: string]: any;
}

/** 成员创建参数 */
export interface MemberCreateParams {
  name: string;
  department_id?: number;
  mobile?: string;
  email?: string;
  title?: string;
  leader_member_id?: number | null;
  sort?: number;
  status?: number; // int16 后端能接 number
  meta?: any; // 发送时可 JSON.stringify
}

/** 成员更新参数（全部可选） */
export type MemberUpdateParams = {
  name?: string;
  department_id?: number | null; // 变更部门，null 表示清空
  mobile?: string | null;
  email?: string | null;
  title?: string | null;
  leader_member_id?: number | null;
  sort?: number | null;
  status?: number | null;
  meta?: any; // 发送时可 JSON.stringify
};

/** 成员查询参数（列表） */
export interface MemberQuery {
  // 常见筛选与分页参数，按需扩展
  department_id?: number;
  keyword?: string; // 按姓名/手机/邮箱模糊查询
  status?: number;
  page?: number;
  page_size?: number;
  // 允许透传更多参数
  [key: string]: any;
}

/** 统一解析：从任意响应结构中解析出成员数组；取不到就返回 [] */
function parseMembersFromResponse(resp: any): Member[] {
  const data = resp?.data ?? resp;
  const list = Array.isArray(data?.list)
    ? data.list
    : Array.isArray(data)
      ? data
      : [];
  return normalizeMembers(list);
}

/** 归一化成员结构，确保字段类型与可用性 */
function normalizeMembers(members: any[]): Member[] {
  return members
    .filter((m) => m && typeof m === "object")
    .map((m) => {
      const item: Member = {
        id: Number(m.id),
        name: String(m.name ?? ""),
        mobile: typeof m.mobile === "string" ? m.mobile : (m.mobile ?? null),
        email: typeof m.email === "string" ? m.email : (m.email ?? null),
        title: typeof m.title === "string" ? m.title : (m.title ?? null),
        department_id:
          typeof m.department_id === "number"
            ? m.department_id
            : (m.department_id ?? null),
        leader_member_id:
          typeof m.leader_member_id === "number" || m.leader_member_id === null
            ? m.leader_member_id
            : null,
        sort: typeof m.sort === "number" ? m.sort : (m.sort ?? null),
        status: typeof m.status === "number" ? m.status : (m.status ?? null),
        meta: m.meta,
        ...m,
      };
      return item;
    });
}

/** 将对象序列化为 URL 查询字符串（简单实现） */
function toQueryString(params?: Record<string, any>): string {
  if (!params || typeof params !== "object") return "";
  const usp = new URLSearchParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v === undefined) return;
    // null 也可透传
    usp.append(k, String(v));
  });
  const qs = usp.toString();
  return qs ? `?${qs}` : "";
}

/** 成员服务 API */
export function useMemberService() {
  const apiClient = useApiClient();
  const baseUrl = "/admin/organization/members";

  return {
    /**
     * 获取成员列表（可带筛选/分页）
     * 统一返回 Member[]
     * 兼容后端返回 { data: { list: [] , total: number } } 或直接返回数组
     */
    getMemberList: async (query?: MemberQuery): Promise<Member[]> => {
      try {
        const res = await apiClient.get<ApiResponse<any>>(
          `${baseUrl}${toQueryString(query)}`
        );
        const serverResp = res?.data ?? res;
        // console.log("获取成员列表成功:", serverResp);
        return parseMembersFromResponse(serverResp);
      } catch (error) {
        console.error("获取成员列表失败:", error);
        return [];
      }
    },

    /**
     * 创建成员
     */
    createMember: async (data: MemberCreateParams): Promise<Member | null> => {
      try {
        const payload = {
          ...data,
          // 后端若需要字符串化 meta，这里统一处理
          ...(data?.meta !== undefined
            ? {
                meta:
                  typeof data.meta === "string"
                    ? data.meta
                    : JSON.stringify(data.meta),
              }
            : {}),
        };
        const res = await apiClient.post<ApiResponse<Member>>(baseUrl, payload);
        const serverResp = res?.data ?? res;
        return serverResp?.data ?? null;
      } catch (error) {
        console.error("创建成员失败:", error);
        return null;
      }
    },

    /**
     * 更新成员
     */
    updateMember: async (
      id: number,
      data: MemberUpdateParams
    ): Promise<Member | null> => {
      try {
        const payload = {
          ...data,
          ...(data?.meta !== undefined
            ? {
                meta:
                  typeof data.meta === "string"
                    ? data.meta
                    : JSON.stringify(data.meta),
              }
            : {}),
        };
        const res = await apiClient.put<ApiResponse<Member>>(
          `${baseUrl}/${id}`,
          payload
        );
        const serverResp = res?.data ?? res;
        return serverResp?.data ?? null;
      } catch (error) {
        console.error("更新成员失败:", error);
        return null;
      }
    },

    /**
     * 删除成员
     */
    deleteMember: async (id: number): Promise<boolean> => {
      try {
        await apiClient.delete<ApiResponse<null>>(`${baseUrl}/${id}`);
        return true;
      } catch (error) {
        console.error("删除成员失败:", error);
        return false;
      }
    },

    /**
     * 获取指定成员信息
     */
    getMember: async (id: number): Promise<Member | null> => {
      try {
        const res = await apiClient.get<ApiResponse<Member>>(
          `${baseUrl}/${id}`
        );
        const serverResp = res?.data ?? res;
        return serverResp?.data ?? null;
      } catch (error) {
        console.error("获取成员信息失败:", error);
        return null;
      }
    },
  };
}
