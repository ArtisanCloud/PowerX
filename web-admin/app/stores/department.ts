import { defineStore } from "pinia";
import type { Department } from "~/composables/api/services/departmentService";
import { useDepartmentService } from "~/composables/api/services/departmentService";

type FetchStatus = "idle" | "loading" | "success" | "error";

export const useDepartmentStore = defineStore("department", {
  state: () => ({
    tree: [] as Department[], // 部门树
    flat: [] as Department[], // 扁平化（可选，看你是否需要）
    status: "idle" as FetchStatus,
    error: null as string | null,
    lastFetchedAt: null as number | null,
  }),

  getters: {
    // 例：根据 id 快速查找
    byId: (state) => {
      const map = new Map<number, Department>();
      state.flat.forEach((d) => map.set(d.id, d));
      return (id: number) => map.get(id) || null;
    },

    // 获取根部门
    rootDepartments: (state) => {
      return state.tree.filter((d) => !d.parent_id);
    },

    // 获取指定部门的子部门
    getChildren: (state) => {
      return (parentId: number) =>
        state.flat.filter((d) => d.parent_id === parentId);
    },
  },

  actions: {
    // 拉取树（带简单的缓存策略，可按需调整）
    async fetchTree({ force = false }: { force?: boolean } = {}) {
      if (this.status === "loading") return;
      if (
        !force &&
        this.lastFetchedAt &&
        Date.now() - this.lastFetchedAt < 30_000
      ) {
        // 30 秒内已拉过，直接复用
        return;
      }

      const api = useDepartmentService();
      this.status = "loading";
      this.error = null;

      try {
        const tree = await api.getDepartmentTree();
        this.tree = tree || [];
        this.flat = flattenTree(this.tree);
        this.status = "success";
        this.lastFetchedAt = Date.now();
      } catch (e: any) {
        this.status = "error";
        this.error = e?.message || "加载部门失败";
        throw e;
      }
    },

    // 创建部门
    async createDepartment(payload: { name: string; parent_id?: number }) {
      const api = useDepartmentService();
      try {
        await api.createDepartment(payload);
        await this.fetchTree({ force: true }); // 简单粗暴，保证一致性
      } catch (error) {
        console.error("创建部门失败:", error);
        throw error;
      }
    },

    // 更新部门
    async updateDepartment(id: number, payload: Partial<Department>) {
      const api = useDepartmentService();
      try {
        await api.updateDepartment(id, payload);
        await this.fetchTree({ force: true });
      } catch (error) {
        console.error("更新部门失败:", error);
        throw error;
      }
    },

    // 删除部门
    async deleteDepartment(id: number) {
      const api = useDepartmentService();
      try {
        await api.deleteDepartment(id);
        await this.fetchTree({ force: true });
      } catch (error) {
        console.error("删除部门失败:", error);
        throw error;
      }
    },

    // 可选：单个详情（会同时同步到 flat）
    async fetchOne(id: number) {
      const api = useDepartmentService();
      try {
        const d = await api.getDepartment?.(id);
        if (!d) return null;
        // 更新 flat
        const idx = this.flat.findIndex((x) => x.id === id);
        if (idx >= 0) this.flat[idx] = d;
        else this.flat.push(d);
        return d;
      } catch (error) {
        console.error("获取部门详情失败:", error);
        return null;
      }
    },

    // 可选：本地新增/更新/删除，结合接口成功后再同步
    upsertLocal(dept: Department) {
      const idx = this.flat.findIndex((x) => x.id === dept.id);
      if (idx >= 0) this.flat[idx] = { ...this.flat[idx], ...dept };
      else this.flat.push(dept);
    },

    // 清空（例如退出登录）
    resetAll() {
      this.$reset();
    },
  },
});

// 工具：扁平化
function flattenTree(nodes: Department[] = []): Department[] {
  const out: Department[] = [];
  const walk = (arr: Department[], parentId: number | null = null) => {
    for (const n of arr) {
      out.push({ ...n, parent_id: n.parent_id ?? parentId });
      if (n.children?.length) walk(n.children, n.id);
    }
  };
  walk(nodes);
  return out;
}
