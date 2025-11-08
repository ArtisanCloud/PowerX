<script setup lang="ts">
import { onMounted } from "vue";
import { useUserManagement } from "~/composables/useUserManagement";

// 使用用户管理 composable
const {
  users,
  loading,
  error,
  pagination,
  paginationInfo,
  filters,
  hasUsers,
  isEmpty,
  hasNextPage,
  hasPrevPage,
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
  toggleUserStatus,
} = useUserManagement();

// 页面加载时获取用户列表
onMounted(() => {
  fetchUsers();
});

// 搜索处理
const handleSearch = (query: string) => {
  searchUsers(query);
};

// 筛选处理
const handleFilter = (filterType: string, value: any) => {
  filterUsers({ [filterType]: value });
};

// 分页处理
const handlePageChange = (page: number) => {
  changePage(page);
};

const handlePageSizeChange = (pageSize: number) => {
  changePageSize(pageSize);
};

// 排序处理
const handleSort = (column: string) => {
  const currentOrder = filters.sortBy === column && filters.sortOrder === "asc" ? "desc" : "asc";
  sortUsers(column, currentOrder);
};

// 用户操作
const handleCreateUser = async (userData: any) => {
  try {
    await createUser(userData);
    // 显示成功消息
  } catch (error) {
    // 显示错误消息
  }
};

const handleUpdateUser = async (id: string, userData: any) => {
  try {
    await updateUser(id, userData);
    // 显示成功消息
  } catch (error) {
    // 显示错误消息
  }
};

const handleDeleteUser = async (id: string) => {
  if (confirm("确定要删除此用户吗？")) {
    try {
      await deleteUser(id);
      // 显示成功消息
    } catch (error) {
      // 显示错误消息
    }
  }
};

const handleToggleStatus = async (user: any) => {
  const newStatus = user.status === "active" ? "inactive" : "active";
  try {
    await toggleUserStatus(user.id, newStatus);
    // 显示成功消息
  } catch (error) {
    // 显示错误消息
  }
};
</script>

<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900 mb-2">用户管理示例</h1>
      <p class="text-gray-600">展示如何使用带分页的用户管理功能</p>
    </div>

    <!-- 搜索和筛选 -->
    <div class="mb-6 bg-white p-4 rounded-lg shadow-sm">
      <div class="flex flex-wrap gap-4 items-end">
        <!-- 搜索框 -->
        <div class="flex-grow min-w-[200px]">
          <UInput
            :model-value="filters.search"
            icon="i-heroicons-magnifying-glass"
            placeholder="搜索用户..."
            @update:model-value="handleSearch"
          />
        </div>

        <!-- 部门筛选 -->
        <div class="w-40">
          <USelect
            :model-value="filters.department"
            placeholder="选择部门"
            :options="[
              { label: '全部部门', value: undefined },
              { label: '技术部', value: '技术部' },
              { label: '市场部', value: '市场部' },
              { label: '销售部', value: '销售部' },
            ]"
            @update:model-value="(value) => handleFilter('department', value)"
          />
        </div>

        <!-- 角色筛选 -->
        <div class="w-40">
          <USelect
            :model-value="filters.role"
            placeholder="选择角色"
            :options="[
              { label: '全部角色', value: undefined },
              { label: '管理员', value: '管理员' },
              { label: '编辑', value: '编辑' },
              { label: '用户', value: '用户' },
            ]"
            @update:model-value="(value) => handleFilter('role', value)"
          />
        </div>

        <!-- 状态筛选 -->
        <div class="w-40">
          <USelect
            :model-value="filters.status"
            placeholder="选择状态"
            :options="[
              { label: '全部状态', value: undefined },
              { label: '激活', value: 'active' },
              { label: '禁用', value: 'inactive' },
            ]"
            @update:model-value="(value) => handleFilter('status', value)"
          />
        </div>

        <!-- 重置按钮 -->
        <UButton
          color="neutral"
          variant="ghost"
          icon="i-heroicons-arrow-path"
          @click="resetFilters"
        >
          重置
        </UButton>
      </div>
    </div>

    <!-- 加载状态 -->
    <div v-if="loading" class="flex justify-center py-8">
      <UIcon name="i-heroicons-arrow-path" class="w-6 h-6 animate-spin" />
      <span class="ml-2">加载中...</span>
    </div>

    <!-- 错误状态 -->
    <div v-else-if="error" class="bg-red-50 border border-red-200 rounded-lg p-4 mb-6">
      <div class="flex items-center">
        <UIcon name="i-heroicons-exclamation-triangle" class="w-5 h-5 text-red-500 mr-2" />
        <span class="text-red-700">{{ error }}</span>
      </div>
    </div>

    <!-- 用户列表 -->
    <div v-else-if="hasUsers" class="bg-white rounded-lg shadow-sm">
      <!-- 列表头部信息 -->
      <div class="px-6 py-4 border-b border-gray-200 flex justify-between items-center">
        <div class="text-sm text-gray-600">
          显示第 {{ paginationInfo.start }} - {{ paginationInfo.end }} 条，
          共 {{ paginationInfo.total }} 条记录
        </div>
        <div class="flex items-center gap-2">
          <span class="text-sm text-gray-600">每页显示：</span>
          <USelect
            :model-value="pagination.pageSize"
            :options="[
              { label: '10', value: 10 },
              { label: '20', value: 20 },
              { label: '50', value: 50 },
              { label: '100', value: 100 },
            ]"
            @update:model-value="handlePageSizeChange"
            class="w-20"
          />
        </div>
      </div>

      <!-- 表格 -->
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead class="bg-gray-50">
            <tr>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                <button
                  @click="handleSort('name')"
                  class="flex items-center hover:text-gray-700"
                >
                  姓名
                  <UIcon
                    v-if="filters.sortBy === 'name'"
                    :name="filters.sortOrder === 'asc' ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'"
                    class="w-4 h-4 ml-1"
                  />
                </button>
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                用户名
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                邮箱
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                部门
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                角色
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                状态
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                操作
              </th>
            </tr>
          </thead>
          <tbody class="bg-white divide-y divide-gray-200">
            <tr v-for="user in users" :key="user.id" class="hover:bg-gray-50">
              <td class="px-6 py-4 whitespace-nowrap">
                <div class="flex items-center">
                  <UAvatar :src="user.avatar" :alt="user.name" size="sm" class="mr-3" />
                  <span class="text-sm font-medium text-gray-900">{{ user.name }}</span>
                </div>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                {{ user.username }}
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                {{ user.email }}
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                {{ user.department }}
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                {{ user.role }}
              </td>
              <td class="px-6 py-4 whitespace-nowrap">
                <UBadge
                  :color="user.status === 'active' ? 'success' : 'neutral'"
                  variant="subtle"
                  size="sm"
                >
                  {{ user.status === 'active' ? '激活' : '禁用' }}
                </UBadge>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-sm font-medium">
                <div class="flex gap-2">
                  <UButton
                    size="xs"
                    variant="ghost"
                    icon="i-heroicons-pencil-square"
                    @click="handleUpdateUser(user.id, user)"
                  >
                    编辑
                  </UButton>
                  <UButton
                    size="xs"
                    :color="user.status === 'active' ? 'warning' : 'success'"
                    variant="ghost"
                    :icon="user.status === 'active' ? 'i-heroicons-lock-closed' : 'i-heroicons-lock-open'"
                    @click="handleToggleStatus(user)"
                  >
                    {{ user.status === 'active' ? '禁用' : '启用' }}
                  </UButton>
                  <UButton
                    size="xs"
                    color="error"
                    variant="ghost"
                    icon="i-heroicons-trash"
                    @click="handleDeleteUser(user.id)"
                  >
                    删除
                  </UButton>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 分页 -->
      <div class="px-6 py-4 border-t border-gray-200 flex justify-between items-center">
        <div class="text-sm text-gray-600">
          第 {{ pagination.page }} 页，共 {{ pagination.totalPages }} 页
        </div>
        <div class="flex gap-2">
          <UButton
            :disabled="!hasPrevPage"
            variant="outline"
            size="sm"
            icon="i-heroicons-chevron-left"
            @click="handlePageChange(pagination.page - 1)"
          >
            上一页
          </UButton>
          
          <!-- 页码按钮 -->
          <template v-for="page in Math.min(5, pagination.totalPages)" :key="page">
            <UButton
              v-if="Math.abs(page - pagination.page) <= 2 || page === 1 || page === pagination.totalPages"
              :variant="page === pagination.page ? 'solid' : 'outline'"
              size="sm"
              @click="handlePageChange(page)"
            >
              {{ page }}
            </UButton>
          </template>
          
          <UButton
            :disabled="!hasNextPage"
            variant="outline"
            size="sm"
            icon="i-heroicons-chevron-right"
            @click="handlePageChange(pagination.page + 1)"
          >
            下一页
          </UButton>
        </div>
      </div>
    </div>

    <!-- 空状态 -->
    <div v-else-if="isEmpty" class="text-center py-12 bg-gray-50 rounded-lg">
      <UIcon name="i-heroicons-user-slash" class="w-12 h-12 text-gray-400 mx-auto mb-4" />
      <h3 class="text-lg font-medium text-gray-900 mb-2">暂无用户</h3>
      <p class="text-gray-500 mb-4">
        {{ filters.search || filters.department || filters.role || filters.status 
           ? '没有找到符合条件的用户' 
           : '还没有创建任何用户' }}
      </p>
      <div class="flex justify-center gap-3">
        <UButton color="primary" @click="handleCreateUser({})">
          添加用户
        </UButton>
        <UButton
          v-if="filters.search || filters.department || filters.role || filters.status"
          color="neutral"
          variant="outline"
          @click="resetFilters"
        >
          重置筛选
        </UButton>
      </div>
    </div>
  </div>
</template>