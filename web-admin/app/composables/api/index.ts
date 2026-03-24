import type {
  ApiRequestConfig,
  HttpMethod,
  RequestInterceptor,
  ResponseInterceptor,
} from "./types/types";

// ✅ 引入你的全局 Loading 状态（只改 visible，不动手动锁屏逻辑）
import {
  useGlobalLoading,
  useGL_AutoVisible,
  useGL_ReqPending,
} from "~/composables/useGlobalLoading";

/** =========================
 * API 客户端配置
 * ======================== */
interface ApiClientConfig {
  baseURL: string;
  timeout?: number;
  headers?: Record<string, string>;
  requestInterceptors?: RequestInterceptor[];
  responseInterceptors?: ResponseInterceptor[];
}

let refreshingAccessTokenPromise: Promise<string | null> | null = null;

const isAuthRefreshEndpoint = (url?: string): boolean => {
  const u = String(url || "");
  return /\/admin\/user\/auth\/refresh(?:\?|$)/.test(u);
};

const getErrorStatusCode = (error: any): number => {
  const direct = Number(
    error?.status || error?.statusCode || error?.response?.status || 0
  );
  if (direct > 0) return direct;
  const cause = error?.cause;
  return Number(
    cause?.status || cause?.statusCode || cause?.response?.status || 0
  );
};

const clearClientAuthStorage = () => {
  if (!process.client) return;
  [
    "access_token",
    "refresh_token",
    "token_type",
    "expires_in",
    "expires_at",
    "scope",
    "px_current_tenant_uuid",
  ].forEach((k) => localStorage.removeItem(k));
};

const updateClientAuthStorage = (payload: any) => {
  if (!process.client || !payload?.access_token) return;

  const tokenType = String(payload.token_type || "Bearer");
  const expiresInSec = Math.max(0, Number(payload.expires_in || 0));
  const expiresAt = Date.now() + expiresInSec * 1000;

  localStorage.setItem("access_token", String(payload.access_token));
  localStorage.setItem("token_type", tokenType);
  localStorage.setItem("expires_in", String(expiresInSec));
  localStorage.setItem("expires_at", String(expiresAt));
  if (typeof payload.scope === "string") {
    localStorage.setItem("scope", payload.scope);
  }
  if (typeof payload.refresh_token === "string" && payload.refresh_token) {
    localStorage.setItem("refresh_token", payload.refresh_token);
  }
};

const isTokenExpiredByLocalClock = (): boolean => {
  if (!process.client) return true;
  const expiresAtRaw = localStorage.getItem("expires_at");
  if (!expiresAtRaw) return true;
  const expiresAt = Number(expiresAtRaw);
  if (!Number.isFinite(expiresAt) || expiresAt <= 0) return true;
  return Date.now() >= expiresAt;
};

const tryRefreshAccessToken = async (): Promise<string | null> => {
  if (!process.client) return null;
  if (refreshingAccessTokenPromise) return refreshingAccessTokenPromise;

  const refreshToken = localStorage.getItem("refresh_token");
  if (!refreshToken) return null;

  refreshingAccessTokenPromise = (async () => {
    try {
      const response: any = await $fetch("/api/admin/user/auth/refresh", {
        method: "POST",
        body: { refresh_token: refreshToken },
        headers: {
          "Content-Type": "application/json",
          Accept: "application/json",
        },
        timeout: globalConfig.timeout,
      });

      const payload = response?.data ?? response;
      if (!payload?.access_token) {
        throw new Error("refresh response missing access_token");
      }

      updateClientAuthStorage(payload);
      return String(payload.access_token);
    } catch {
      clearClientAuthStorage();
      return null;
    } finally {
      refreshingAccessTokenPromise = null;
    }
  })();

  return refreshingAccessTokenPromise;
};

/**
 * 小贴士：可以在 ./types/types 里给 ApiRequestConfig 扩展可选字段
 * interface ApiRequestConfig {
 *   useGlobalLoading?: boolean;  // 默认 true：显示全局 Loading
 *   loadingMessage?: string;     // 单次请求的临时文案
 * }
 */

// 全局配置（可通过 setApiConfig 动态修改）
let globalConfig: ApiClientConfig = {
  baseURL: "/api",
  timeout: 30000,
  headers: {
    "Content-Type": "application/json",
    Accept: "application/json",
  },
  requestInterceptors: [
    // --- 认证头 ---
    {
      onRequest: async (config) => {
        // 自动添加认证头（仅客户端）
        if (process.client && !config.skipAuth) {
          let token = localStorage.getItem("access_token");
          if ((!token || isTokenExpiredByLocalClock()) && !isAuthRefreshEndpoint((config as any).url)) {
            token = await tryRefreshAccessToken();
          }
          const tokenType = localStorage.getItem("token_type") || "Bearer";
          if (token) {
            config.headers = {
              ...config.headers,
              Authorization: `${tokenType} ${token}`,
            };
          }
        }
        return config;
      },
    },
    // --- JWT-only 防回归护栏：移除禁用租户头 ---
    {
      onRequest: async (config) => {
        if (!config.headers) {
          return config;
        }

        const headers = config.headers as Record<string, string | undefined>;
        const hadLegacyHeader =
          typeof headers["tenant-uuid"] !== "undefined";

        if (hadLegacyHeader) {
          delete headers["tenant-uuid"];

          if (process.dev) {
            console.warn(
              "[api] removed legacy tenant header; tenant must come from JWT claims"
            );
          }
        }

        config.headers = headers;
        return config;
      },
    },
    // --- 全局 Loading：请求开始 +1，显示 autoVisible ---
    {
      onRequest: async (config) => {
        if (!process.client) return config;

        // 默认开启；显式传 false 关闭
        const enable =
          typeof (config as any).useGlobalLoading === "undefined"
            ? true
            : (config as any).useGlobalLoading !== false;

        if (enable) {
          const reqPending = useGL_ReqPending();
          const auto = useGL_AutoVisible();
          const gl = useGlobalLoading();

          reqPending.value += 1;
          auto.value = true; // 只要有请求在飞就显示
          if ((config as any).loadingMessage) {
            gl.setMessage(String((config as any).loadingMessage));
          }
        }
        return config;
      },
    },
  ],
  responseInterceptors: [
    // --- 全局 Loading：请求结束 -1，计数为 0 时关闭 autoVisible ---
    {
      onResponse: async (response) => {
        if (process.client) {
          const reqPending = useGL_ReqPending();
          const auto = useGL_AutoVisible();
          const gl = useGlobalLoading();

          reqPending.value = Math.max(0, reqPending.value - 1);
          if (reqPending.value === 0) {
            auto.value = false;
            gl.setProgress(undefined);
          }
        }
        return response;
      },
      onResponseError: async (error) => {
        if (process.client) {
          const reqPending = useGL_ReqPending();
          const auto = useGL_AutoVisible();
          const gl = useGlobalLoading();

          reqPending.value = Math.max(0, reqPending.value - 1);
          if (reqPending.value === 0) {
            auto.value = false;
            gl.setProgress(undefined);
          }
        }
        // 不吞错，抛给上层（normalizeApiError）
        throw error;
      },
    },
  ],
};

/**
 * 设置全局 API 配置（浅合并 + headers 深合并）
 */
export const setApiConfig = (config: Partial<ApiClientConfig>) => {
  const nextRequestInterceptors =
    config.requestInterceptors && config.requestInterceptors.length
      ? [...(globalConfig.requestInterceptors || []), ...config.requestInterceptors]
      : globalConfig.requestInterceptors;

  const nextResponseInterceptors =
    config.responseInterceptors && config.responseInterceptors.length
      ? [...(globalConfig.responseInterceptors || []), ...config.responseInterceptors]
      : globalConfig.responseInterceptors;

  globalConfig = {
    ...globalConfig,
    ...config,
    headers: {
      ...globalConfig.headers,
      ...config.headers,
    },
    requestInterceptors: nextRequestInterceptors,
    responseInterceptors: nextResponseInterceptors,
  };
};

/** =========================
 * 工具函数
 * ======================== */

/**
 * 将 params 拼接到 URL
 */
const handleUrl = (url: string, params?: Record<string, any>): string => {
  if (!params) return url;
  const queryString = Object.entries(params)
    .filter(([, value]) => value !== undefined && value !== null)
    .map(([key, value]) => {
      if (Array.isArray(value)) {
        return value
          .map((v) => `${encodeURIComponent(key)}=${encodeURIComponent(v)}`)
          .join("&");
      }
      return `${encodeURIComponent(key)}=${encodeURIComponent(value)}`;
    })
    .join("&");
  return queryString
    ? `${url}${url.includes("?") ? "&" : "?"}${queryString}`
    : url;
};

/**
 * 判断是否为可直接作为 body 传递的“原生”类型（不要 JSON.stringify）
 */
const isNativeBody = (data: any) =>
  (typeof FormData !== "undefined" && data instanceof FormData) ||
  (typeof Blob !== "undefined" && data instanceof Blob) ||
  (typeof ArrayBuffer !== "undefined" && data instanceof ArrayBuffer) ||
  (typeof URLSearchParams !== "undefined" && data instanceof URLSearchParams) ||
  // Node 端的 stream/Buffer 也让 $fetch 处理
  (typeof ReadableStream !== "undefined" && data instanceof ReadableStream);

/**
 * 运行请求拦截器链
 */
const runRequestInterceptors = async (cfg: ApiRequestConfig) => {
  let c = cfg;
  for (const itc of globalConfig.requestInterceptors || []) {
    if (itc.onRequest) {
      c = await itc.onRequest(c);
    }
  }
  return c;
};

/**
 * 运行响应拦截器链
 */
const applyResponseInterceptors = async (response: any): Promise<any> => {
  let result = response;
  for (const itc of globalConfig.responseInterceptors || []) {
    if (itc.onResponse) {
      result = await itc.onResponse(result);
    }
  }
  return result;
};

/**
 * 运行错误拦截器链（允许拦截器「吞错」或转换）
 */
const applyErrorInterceptors = async (error: any): Promise<any> => {
  let result = error;
  for (const itc of globalConfig.responseInterceptors || []) {
    if (itc.onResponseError) {
      try {
        const maybeHandled = await itc.onResponseError(result);
        // 如果返回值与入参不同，认为已处理，直接返回
        if (maybeHandled !== result) {
          return maybeHandled;
        }
      } catch (e) {
        result = e;
      }
    }
  }
  // ofetch/$fetch 超时/主动取消会抛 AbortError（或作为 cause），此时没有 response
  const isAbort =
    result?.name === "AbortError" ||
    result?.cause?.name === "AbortError" ||
    String(result?.message || "").toLowerCase().includes("aborted") ||
    String(result?.message || "").toLowerCase().includes("timeout");
  if (!result || !result.response) {
    if (isAbort) {
      throw new Error("请求超时，请稍后重试");
    }
    throw new Error("网络错误，请检查网络连接");
  }

  // 统一把后端 ErrorResponse.error 显示出来，避免前端只看到 “400 Bad Request”
  const data = (result && (result.data ?? result.response?._data ?? result.response?.data)) as any;
  if (data) {
    let msg = "";
    if (typeof data === "string") {
      msg = data;
    } else if (typeof data === "object") {
      msg =
        (typeof data.error === "string" && data.error) ||
        (typeof data.message === "string" && data.message) ||
        (typeof data.detail === "string" && data.detail) ||
        "";
    }
    msg = String(msg || "").trim();
    if (msg) {
      const e = new Error(msg);
      (e as any).cause = result;
      throw e;
    }
  }

  throw result;
};

/**
 * 组装请求：method/url/body/headers 等
 */
const handleRequestConfig = async (
  method: HttpMethod,
  url: string,
  data?: any,
  config: ApiRequestConfig = {}
): Promise<{
  url: string;
  fetchOptions: any;
  finalConfig: ApiRequestConfig;
}> => {
  // 合并 headers
  const headers: Record<string, string> = {
    ...globalConfig.headers,
    ...config.headers,
  };

  // 拼 URL（baseURL + params + GET/DELETE 数据上屏）
  let fullUrl = url.startsWith("http") ? url : `${globalConfig.baseURL}${url}`;

  // 先处理 config.params
  if (config.params) {
    fullUrl = handleUrl(fullUrl, config.params);
  }

  // 处理 data：GET/DELETE => query；其他 => body
  let body: any = undefined;
  if (data !== undefined) {
    if (method === "GET" || method === "DELETE") {
      fullUrl = handleUrl(fullUrl, data);
    } else {
      body = isNativeBody(data) ? data : JSON.stringify(data);
      // 如果是原生体，移除 Content-Type，让运行时自动设置
      if (isNativeBody(data) && headers["Content-Type"]) {
        delete headers["Content-Type"];
      }
    }
  }

  // 构造 ApiRequestConfig，进入拦截器
  let mergedConfig: ApiRequestConfig = {
    ...config,
    method,
    headers,
    body,
  };

  (mergedConfig as any).url = fullUrl;

  // 请求拦截器
  mergedConfig = await runRequestInterceptors(mergedConfig);

  // 从 mergedConfig 提取给 $fetch 的 options
  const {
    params, // 已经处理
    useGlobalError, // 仅透传，不在此实现
    useGlobalLoading, // 已由拦截器处理，不透传给 $fetch
    loadingMessage, // 已由拦截器处理
    skipAuth, // 已被 onRequest 使用
    // 其余透传字段不破坏
    ...rest
  } = mergedConfig as any;

  const fetchOptions = {
    method: method as any,
    body: mergedConfig.body,
    headers: mergedConfig.headers,
    // 超时（ofetch 支持 timeout 毫秒）
    timeout: globalConfig.timeout,
    // 允许调用方透传 ofetch 其它可选项（如 responseType）
    ...rest,
  };

  return { url: fullUrl, fetchOptions, finalConfig: mergedConfig };
};

/** =========================
 * API 客户端（$fetch 版）
 * ======================== */

export const useApiClient = () => {
  /**
   * 核心请求方法（统一使用 $fetch）
   */
  const request = async <T = any>(
    method: HttpMethod,
    url: string,
    data?: any,
    config: ApiRequestConfig = {}
  ) => {
    try {
      const { url: fullUrl, fetchOptions } = await handleRequestConfig(
        method,
        url,
        data,
        config
      );

      // 直接用 Nuxt 的全局 $fetch
      // - 成功：返回已解析的数据（JSON 自动解析）
      // - 失败：抛出 FetchError，内含 response/status 等
      const responseData = await $fetch(fullUrl, {
        ...fetchOptions,
      });

      // 应用响应拦截器
      const result = await applyResponseInterceptors(responseData);
      return result as T;
    } catch (err: any) {
      const retried = Boolean((config as any).__authRetried);
      const statusCode = getErrorStatusCode(err);
      const shouldTryRefresh =
        process.client &&
        !config.skipAuth &&
        !retried &&
        statusCode === 401 &&
        !isAuthRefreshEndpoint(url);

      if (shouldTryRefresh) {
        const refreshedToken = await tryRefreshAccessToken();
        if (refreshedToken) {
          return request<T>(method, url, data, {
            ...(config as any),
            __authRetried: true,
          });
        }
      }

      // 统一交给错误拦截器处理/转换/抛出
      return applyErrorInterceptors(err);
    }
  };

  /** 便捷方法族 */
  const get = <T = any>(url: string, config?: ApiRequestConfig) => {
    return request<T>("GET", url, undefined, config as any);
  };

  const post = <T = any>(
    url: string,
    data?: any,
    config?: ApiRequestConfig
  ) => {
    return request<T>("POST", url, data, config as any);
  };

  const put = <T = any>(url: string, data?: any, config?: ApiRequestConfig) => {
    return request<T>("PUT", url, data, config as any);
  };

  const del = <T = any>(url: string, config?: ApiRequestConfig) => {
    return request<T>("DELETE", url, undefined, config as any);
  };

  const patch = <T = any>(
    url: string,
    data?: any,
    config?: ApiRequestConfig
  ) => {
    return request<T>("PATCH", url, data, config as any);
  };

  /**
   * 上传文件（FormData/Blob 等）
   * - 不主动设置 Content-Type，让运行时自动带上 multipart 边界
   */
  const upload = <T = any>(
    url: string,
    formData: FormData,
    config?: ApiRequestConfig
  ) => {
    const uploadConfig: ApiRequestConfig = {
      ...config,
      headers: {
        ...config?.headers,
      },
    };
    if (uploadConfig.headers && "Content-Type" in uploadConfig.headers) {
      delete (uploadConfig.headers as any)["Content-Type"];
    }
    return request<T>("POST", url, formData, uploadConfig as any);
  };

  // 统一解包 data（适配后端 SuccessResponse/ResponseList）
  const unwrap = <T = any>(resp: any): T =>
    resp && typeof resp === "object" && "data" in resp ? (resp as any).data : resp;

  // 专用于列表：返回 { items, pagination }
  const unwrapList = <T = any>(resp: any): { items: T[]; pagination?: any } => {
    const unwrapped: any = unwrap(resp) || {};
    const items = Array.isArray(unwrapped.items) ? (unwrapped.items as T[]) : [];
    const pagination = unwrapped.pagination || {};
    return { items, pagination };
  };

  return {
    request,
    get,
    post,
    put,
    delete: del,
    patch,
    upload,
    unwrap,
    unwrapList,
  };
};
