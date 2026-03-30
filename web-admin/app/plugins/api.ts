// app/plugins/api.ts
import { defineNuxtPlugin } from "#app";
import { setApiConfig } from "~/composables/api";
import { useAuth } from "~/composables/useAuth";

export default defineNuxtPlugin(() => {
  const config = useRuntimeConfig();
  const { getToken, isTokenExpired, clearAuth } = useAuth();
  const loadSetupStatus = async (): Promise<{
    configured: boolean;
    requires_login: boolean;
  } | null> => {
    try {
      const resp: any = await $fetch("/api/v1/admin/setup/status", {
        method: "GET",
        timeout: 5000,
      });
      const payload = resp?.data ?? resp;
      return {
        configured: Boolean(payload?.configured),
        requires_login: Boolean(payload?.requires_login),
      };
    } catch {
      return null;
    }
  };
  const resolveStatusCode = (error: any): number => {
    const direct = Number(
      error?.response?.status ||
        error?.status ||
        error?.statusCode ||
        error?.response?._data?.status ||
        0
    );
    if (direct > 0) return direct;
    const cause = error?.cause;
    return Number(
      cause?.response?.status ||
        cause?.status ||
        cause?.statusCode ||
        cause?.response?._data?.status ||
        0
    );
  };

  setApiConfig({
    baseURL: config.public.apiBase || "/api",
    // 默认超时：Agent 会话/配置拉取在本地冷启动或 DB 慢查询时可能超过 10s，过短会被浏览器 Abort 掉并误报“网络错误”
    timeout: 30000,
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },

    // 请求拦截器
    requestInterceptors: [
      {
        onRequest: (req) => {
          // 登录、注册、刷新 token 等接口直接跳过
          if (req.skipAuth) {
            return req;
          }

          const token = getToken();

          // 只有有 token 才加上 Authorization，没有 token 不抛错
          if (token && !isTokenExpired()) {
            req.headers = {
              ...req.headers,
              Authorization: `Bearer ${token}`,
            };
          }

          return req;
        },
        onRequestError: (error) => {
          console.error("Request error:", error);
          return Promise.reject(error);
        },
      },
    ],

    // 响应拦截器
    responseInterceptors: [
      {
        onResponse: (res) => res,
        onResponseError: async (error) => {
          const statusCode = resolveStatusCode(error);

          if (statusCode === 401) {
            if (process.client) {
              const router = useRouter();
              const setup = await loadSetupStatus();
              if (setup && !setup.configured && !setup.requires_login) {
                if (!router.currentRoute.value.path.includes("/setup")) {
                  await router.push("/setup");
                }
                return Promise.reject(error);
              }
              clearAuth();
              if (!router.currentRoute.value.path.includes("/users/login")) {
                await router.push("/users/login");
              }
            }
          }

          if (error?.response?.data?.message) {
            console.error("API Error:", error.response.data.message);
          }

          return Promise.reject(error);
        },
      },
    ],
  });
});
