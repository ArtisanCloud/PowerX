import { ref, computed, reactive } from "vue";
import {
  useAuthService,
  type User,
  type UserFilters,
} from "~/composables/api/services/authService";
import type { PaginatedResponse } from "~/composables/api/types/types";

/**
 * 用户管理 Composable
 * 提供用户列表的分页、搜索、筛选功能
 */
export const useUserManagement = () => {
  const authService = useAuthService();

  // 数据状态
  const users = ref<User[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);

  // 分页状态
  const pagination = reactive({
    page: 1,
    pageSize: 10,
    total: 0,
    totalPages: 0,
  });

  // 筛选条件
  const filters = reactive<UserFilters>({
    page: 1,
    pageSize: 10,
    search: "",
    department: undefined,
    role: undefined,
    status: undefined,
    sortBy: "createdAt",
    sortOrder: "desc",
  });

  /**
   * 获取用户列表
   */
  const fetchUsers = async () => {
    try {
      loading.value = true;
      error.value = null;

      // 更新筛选条件中的分页信息
      filters.page = pagination.page;
      filters.pageSize = pagination.pageSize;

      const response = await authService.getUsers(filters);

      if (response.code === 0) {
        users.value = response.data.items;
        pagination.total = response.data.total;
        pagination.totalPages = response.data.totalPages;
      } else {
        throw new Error(response.message);
      }
    } catch (err: any) {
      error.value = err.message || "获取用户列表失败";
      users.value = [];
    } finally {
      loading.value = false;
    }
  };

  /**
   * 搜索用户
   */
  const searchUsers = (query: string) => {
    filters.search = query;
    pagination.page = 1; // 重置到第一页
    fetchUsers();
  };

  /**
   * 筛选用户
   */
  const filterUsers = (newFilters: Partial<UserFilters>) => {
    Object.assign(filters, newFilters);
    pagination.page = 1; // 重置到第一页
    fetchUsers();
  };

  /**
   * 切换页码
   */
  const changePage = (page: number) => {
    pagination.page = page;
    fetchUsers();
  };

  /**
   * 改变每页大小
   */
  const changePageSize = (pageSize: number) => {
    pagination.pageSize = pageSize;
    pagination.page = 1; // 重置到第一页
    fetchUsers();
  };

  /**
   * 排序
   */
  const sortUsers = (sortBy: string, sortOrder: "asc" | "desc" = "asc") => {
    filters.sortBy = sortBy;
    filters.sortOrder = sortOrder;
    fetchUsers();
  };

  /**
   * 重置筛选条件
   */
  const resetFilters = () => {
    filters.search = "";
    filters.department = undefined;
    filters.role = undefined;
    filters.status = undefined;
    filters.sortBy = "createdAt";
    filters.sortOrder = "desc";
    pagination.page = 1;
    fetchUsers();
  };

  /**
   * 创建用户
   */
  const createUser = async (userData: any) => {
    try {
      loading.value = true;
      const response = await authService.createUser(userData);

      if (response.code === 0) {
        await fetchUsers(); // 刷新列表
        return response.data;
      } else {
        throw new Error(response.message);
      }
    } catch (err: any) {
      error.value = err.message || "创建用户失败";
      throw err;
    } finally {
      loading.value = false;
    }
  };

  /**
   * 更新用户
   */
  const updateUser = async (id: string, userData: Partial<User>) => {
    try {
      loading.value = true;
      const response = await authService.updateUser(id, userData);

      if (response.code === 0) {
        await fetchUsers(); // 刷新列表
        return response.data;
      } else {
        throw new Error(response.message);
      }
    } catch (err: any) {
      error.value = err.message || "更新用户失败";
      throw err;
    } finally {
      loading.value = false;
    }
  };

  /**
   * 删除用户
   */
  const deleteUser = async (id: string) => {
    try {
      loading.value = true;
      const response = await authService.deleteUser(id);

      if (response.code === 0) {
        await fetchUsers(); // 刷新列表
      } else {
        throw new Error(response.message);
      }
    } catch (err: any) {
      error.value = err.message || "删除用户失败";
      throw err;
    } finally {
      loading.value = false;
    }
  };

  /**
   * 批量删除用户
   */
  const batchDeleteUsers = async (ids: string[]) => {
    try {
      loading.value = true;
      const response = await authService.batchDeleteUsers(ids);

      if (response.code === 0) {
        await fetchUsers(); // 刷新列表
      } else {
        throw new Error(response.message);
      }
    } catch (err: any) {
      error.value = err.message || "批量删除用户失败";
      throw err;
    } finally {
      loading.value = false;
    }
  };

  /**
   * 切换用户状态
   */
  const toggleUserStatus = async (
    id: string,
    status: "active" | "inactive"
  ) => {
    try {
      loading.value = true;
      const response = await authService.toggleUserStatus(id, status);

      if (response.code === 0) {
        await fetchUsers(); // 刷新列表
        return response.data;
      } else {
        throw new Error(response.message);
      }
    } catch (err: any) {
      error.value = err.message || "切换用户状态失败";
      throw err;
    } finally {
      loading.value = false;
    }
  };

  // 计算属性
  const hasUsers = computed(() => users.value.length > 0);
  const isEmpty = computed(() => !loading.value && users.value.length === 0);
  const hasNextPage = computed(() => pagination.page < pagination.totalPages);
  const hasPrevPage = computed(() => pagination.page > 1);

  // 分页信息
  const paginationInfo = computed(() => {
    const start = (pagination.page - 1) * pagination.pageSize + 1;
    const end = Math.min(
      pagination.page * pagination.pageSize,
      pagination.total
    );
    return {
      start,
      end,
      total: pagination.total,
      page: pagination.page,
      totalPages: pagination.totalPages,
    };
  });

  return {
    // 数据
    users: readonly(users),
    loading: readonly(loading),
    error: readonly(error),

    // 分页
    pagination: readonly(pagination),
    paginationInfo,

    // 筛选
    filters: readonly(filters),

    // 计算属性
    hasUsers,
    isEmpty,
    hasNextPage,
    hasPrevPage,

    // 方法
    fetchUsers,
    searchUsers,
    filterUsers,
    changePage,
    changePageSize,
    sortUsers,
    resetFilters,
    createUser,
    updateUser,
    deleteUser,
    batchDeleteUsers,
    toggleUserStatus,
  };
};

/**
 * 部门管理 Composable
 */
export const useDepartmentManagement = () => {
  const authService = useAuthService();

  const departments = ref([]);
  const loading = ref(false);
  const error = ref<string | null>(null);

  const pagination = reactive({
    page: 1,
    pageSize: 10,
    total: 0,
    totalPages: 0,
  });

  const filters = reactive({
    page: 1,
    pageSize: 10,
    search: "",
    parentId: undefined,
    sortBy: "createdAt",
    sortOrder: "desc" as "asc" | "desc",
  });

  const fetchDepartments = async () => {
    try {
      loading.value = true;
      error.value = null;

      filters.page = pagination.page;
      filters.pageSize = pagination.pageSize;

      const response = await authService.getDepartments(filters);

      if (response.code === 0) {
        departments.value = response.data.items;
        pagination.total = response.data.total;
        pagination.totalPages = response.data.totalPages;
      } else {
        throw new Error(response.message);
      }
    } catch (err: any) {
      error.value = err.message || "获取部门列表失败";
      departments.value = [];
    } finally {
      loading.value = false;
    }
  };

  const fetchDepartmentTree = async () => {
    try {
      loading.value = true;
      const response = await authService.getDepartmentTree();

      if (response.code === 0) {
        return response.data;
      } else {
        throw new Error(response.message);
      }
    } catch (err: any) {
      error.value = err.message || "获取部门树失败";
      throw err;
    } finally {
      loading.value = false;
    }
  };

  const createDepartment = async (data: any) => {
    try {
      loading.value = true;
      const response = await authService.createDepartment(data);

      if (response.code === 0) {
        await fetchDepartments();
        return response.data;
      } else {
        throw new Error(response.message);
      }
    } catch (err: any) {
      error.value = err.message || "创建部门失败";
      throw err;
    } finally {
      loading.value = false;
    }
  };

  const updateDepartment = async (id: string, data: any) => {
    try {
      loading.value = true;
      const response = await authService.updateDepartment(id, data);

      if (response.code === 0) {
        await fetchDepartments();
        return response.data;
      } else {
        throw new Error(response.message);
      }
    } catch (err: any) {
      error.value = err.message || "更新部门失败";
      throw err;
    } finally {
      loading.value = false;
    }
  };

  const deleteDepartment = async (id: string) => {
    try {
      loading.value = true;
      const response = await authService.deleteDepartment(id);

      if (response.code === 0) {
        await fetchDepartments();
      } else {
        throw new Error(response.message);
      }
    } catch (err: any) {
      error.value = err.message || "删除部门失败";
      throw err;
    } finally {
      loading.value = false;
    }
  };

  return {
    departments: readonly(departments),
    loading: readonly(loading),
    error: readonly(error),
    pagination: readonly(pagination),
    filters: readonly(filters),

    fetchDepartments,
    fetchDepartmentTree,
    createDepartment,
    updateDepartment,
    deleteDepartment,
  };
};
