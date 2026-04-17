# API 请求封装模块使用指南

这个模块为 Nuxt.js 应用提供了一个完整的 API 请求封装解决方案，支持类型安全、拦截器、缓存和 SSR。

## 目录结构

```
app/composables/api/
├── index.ts              # 核心 API 客户端
├── types.ts              # 类型定义
├── services/             # API 服务模块
│   ├── userService.ts    # 用户相关 API
│   └── productService.ts # 产品相关 API
└── README.md            # 使用指南
```

## 基础使用

### 1. 在组件中使用 API 服务

```vue
<script setup lang="ts">
// 导入用户服务
const userService = useUserService();

// 登录示例
const loginForm = reactive({
  email: '',
  password: ''
});

const handleLogin = async () => {
  try {
    const response = await userService.login(loginForm);
    console.info('登录成功:', response);
    
    // 保存 token
    const authToken = useCookie('auth_token');
    authToken.value = response.data.token;
    
    // 跳转到仪表板
    await navigateTo('/dashboard');
  } catch (error) {
    console.error('登录失败:', error);
  }
};

// 获取用户列表（支持 SSR）
const { data: users, pending, refresh } = await useAsyncData('users', () => 
  userService.getUsers({ page: 1, pageSize: 10 })
);
</script>

<template>
  <div>
    <!-- 登录表单 -->
    <form @submit.prevent="handleLogin">
      <input v-model="loginForm.email" type="email" placeholder="邮箱" />
      <input v-model="loginForm.password" type="password" placeholder="密码" />
      <button type="submit">登录</button>
    </form>
    
    <!-- 用户列表 -->
    <div v-if="pending">加载中...</div>
    <div v-else-if="users">
      <div v-for="user in users.data.items" :key="user.id">
        {{ user.username }} - {{ user.email }}
      </div>
    </div>
  </div>
</template>
```

### 2. 直接使用 API 客户端

```vue
<script setup lang="ts">
const apiClient = useApiClient();

// 发送 GET 请求
const fetchData = async () => {
  try {
    const response = await apiClient.get('/custom-endpoint');
    console.info(response);
  } catch (error) {
    console.error(error);
  }
};

// 发送 POST 请求
const createData = async (data: any) => {
  try {
    const response = await apiClient.post('/custom-endpoint', data);
    console.info(response);
  } catch (error) {
    console.error(error);
  }
};

// 上传文件
const uploadFile = async (file: File) => {
  const formData = new FormData();
  formData.append('file', file);
  
  try {
    const response = await apiClient.upload('/upload', formData);
    console.info(response);
  } catch (error) {
    console.error(error);
  }
};
</script>
```

## 创建新的 API 服务

### 1. 定义类型

```typescript
// app/composables/api/services/orderService.ts
import { useApiClient } from '../index';
import type { ApiResponse, PaginatedResponse, PaginationParams } from '../types';

// 订单相关类型定义
export interface Order {
  id: string;
  userId: string;
  productId: string;
  quantity: number;
  totalPrice: number;
  status: 'pending' | 'paid' | 'shipped' | 'delivered' | 'cancelled';
  createdAt: string;
  updatedAt: string;
}

export interface OrderCreateParams {
  productId: string;
  quantity: number;
}

export interface OrderUpdateParams {
  status?: Order['status'];
  quantity?: number;
}
```

### 2. 实现服务方法

```typescript
/**
 * 订单服务 API
 */
export const useOrderService = () => {
  const apiClient = useApiClient();
  const baseUrl = '/orders';

  return {
    /**
     * 获取订单列表
     */
    getOrders: (params?: PaginationParams) => {
      return apiClient.get<ApiResponse<PaginatedResponse<Order>>>(baseUrl, { params });
    },

    /**
     * 获取指定订单
     */
    getOrder: (id: string) => {
      return apiClient.get<ApiResponse<Order>>(`${baseUrl}/${id}`);
    },

    /**
     * 创建订单
     */
    createOrder: (data: OrderCreateParams) => {
      return apiClient.post<ApiResponse<Order>>(baseUrl, data);
    },

    /**
     * 更新订单
     */
    updateOrder: (id: string, data: OrderUpdateParams) => {
      return apiClient.put<ApiResponse<Order>>(`${baseUrl}/${id}`, data);
    },

    /**
     * 取消订单
     */
    cancelOrder: (id: string) => {
      return apiClient.patch<ApiResponse<Order>>(`${baseUrl}/${id}/cancel`);
    }
  };
};
```

## 配置和拦截器

API 配置在 `app/plugins/api.ts` 中设置，包括：

- 基础 URL
- 请求超时时间
- 默认请求头
- 请求和响应拦截器

### 自定义拦截器示例

```typescript
// app/plugins/api.ts
export default defineNuxtPlugin((nuxtApp) => {
  setApiConfig({
    baseURL: '/api',
    timeout: 10000,
    requestInterceptors: [
      {
        onRequest: (config) => {
          // 添加认证 token
          const token = useCookie('auth_token').value;
          if (token && !config.skipAuth) {
            config.headers = {
              ...config.headers,
              Authorization: `Bearer ${token}`
            };
          }
          return config;
        }
      }
    ],
    responseInterceptors: [
      {
        onResponseError: (error) => {
          // 处理 401 未授权错误
          if (error?.response?.status === 401) {
            // 清除 token 并重定向到登录页
            const authToken = useCookie('auth_token');
            authToken.value = null;
            navigateTo('/users/login');
          }
          throw error;
        }
      }
    ]
  });
});
```

## 特性

### 1. 类型安全
- 完整的 TypeScript 类型定义
- 自动类型推断
- 编译时类型检查

### 2. SSR 兼容
- 基于 Nuxt 的 `useFetch` 和 `useAsyncData`
- 自动处理服务端和客户端渲染
- 避免重复请求

### 3. 缓存和去重
- 自动请求去重
- 基于 key 的缓存机制
- 支持手动刷新

### 4. 拦截器支持
- 请求拦截器：添加认证、修改请求头等
- 响应拦截器：统一错误处理、数据转换等

### 5. 灵活配置
- 全局配置和请求级配置
- 支持跳过认证、自定义错误处理等
- 文件上传支持

## 最佳实践

1. **按功能模块组织 API 服务**：将相关的 API 请求组织在同一个服务文件中
2. **使用类型定义**：为所有 API 请求和响应定义明确的类型
3. **合理使用缓存**：对于不经常变化的数据使用 `useAsyncData` 进行缓存
4. **统一错误处理**：在拦截器中处理通用错误，如认证失败、网络错误等
5. **环境配置**：根据不同环境配置不同的 API 基础 URL

这个 API 封装模块为您的 Nuxt 应用提供了一个强大、灵活且类型安全的方式来处理后端 API 请求。