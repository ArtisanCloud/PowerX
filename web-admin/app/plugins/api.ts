// app/plugins/api.ts
import { defineNuxtPlugin } from "#app";
import { setApiConfig } from "~/composables/api";
import { useAuth } from "~/composables/useAuth";

export default defineNuxtPlugin(() => {
  const config = useRuntimeConfig();
  const { getToken, isTokenExpired, clearAuth } = useAuth();

  setApiConfig({
    baseURL: config.public.apiBase || "/api",
    timeout: 10000,
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
        onResponseError: (error) => {
          const statusCode = error?.response?.status;

          if (statusCode === 401) {
            // 只有非 skipAuth 的请求才跳转登录页面
            const requireAuth =
              error.config?.headers?.["X-Require-Auth"] === "1" ||
              !error.config?.skipAuth;
            if (requireAuth) {
              clearAuth();
              const router = useRouter();
              router.push("/users/login");
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
