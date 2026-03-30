<script setup lang="ts">
import type { StepperItem } from "@nuxt/ui";
import {
  useSettingsService,
  type SetupStatus,
} from "~/composables/api/services/settingsService";
import { AISettingService } from "~/composables/api/services/aiSettingService";
import TestPanel from "~/components/settings/ai/TestPanel.vue";

definePageMeta({
  layout: false,
});

// Stepper 引用和当前步骤
const stepper = ref();
const currentStep = ref(0);
const isLoading = ref(false);
const completing = ref(false);
const setupStatus = ref<SetupStatus | null>(null);
const settingsService = useSettingsService();
const toast = useToast();
const { locale } = useI18n();
const showLicenseModal = ref(false);
const showTermsModal = ref(false);
const showPrivacyModal = ref(false);
const isZh = computed(() => String(locale.value || "").startsWith("zh"));
const uiCopy = computed(() =>
  isZh.value
    ? {
        pageTitle: "PowerX 系统初始化",
        pageDesc: "欢迎使用 PowerX！请按照以下步骤完成系统初始化设置",
        step1Title: "系统检查 & 许可",
        versionLabel: "当前系统版本：",
        envCheckTitle: "系统环境检测",
        checkLabelMap: {
          database: "数据库连通性",
          storage: "对象存储",
          cache: "缓存服务",
          email: "邮件服务",
          ai: "AI 模型配置",
        } as Record<string, string>,
        checking: "检测中...",
        msgTenantInited: "租户数据已初始化",
        msgTenantNotInited: "租户数据尚未初始化",
        msgStoragePending: "请在部署配置中确认 storage 配置",
        msgCachePending: "请在部署配置中确认 redis 配置",
        msgEmailPending: "可选项，可稍后配置",
        msgAIOk: "已存在 AI 模型配置",
        msgAINone: "尚未配置 AI 模型",
        msgCheckFailed: "检测失败，请检查后端服务与数据库连接",
      }
    : {
        pageTitle: "PowerX Setup",
        pageDesc: "Welcome to PowerX. Complete the steps below to initialize the system.",
        step1Title: "System Check & License",
        versionLabel: "Current version: ",
        envCheckTitle: "Environment Checks",
        checkLabelMap: {
          database: "Database Connectivity",
          storage: "Object Storage",
          cache: "Cache Service",
          email: "Email Service",
          ai: "AI Model Setup",
        } as Record<string, string>,
        checking: "Checking...",
        msgTenantInited: "Tenant data initialized",
        msgTenantNotInited: "Tenant data not initialized",
        msgStoragePending: "Confirm storage config in deployment settings",
        msgCachePending: "Confirm redis config in deployment settings",
        msgEmailPending: "Optional, can be configured later",
        msgAIOk: "AI model configuration detected",
        msgAINone: "AI model is not configured",
        msgCheckFailed: "Check failed. Verify backend service and database connection.",
      }
);
const stepperItems = computed<StepperItem[]>(() => [
  {
    slot: "system-check" as const,
    title: isZh.value ? "系统检查 & 许可" : "System Check & License",
    description: isZh.value ? "检测系统环境和确认使用条款" : "Validate environment and accept terms",
    icon: "i-lucide-shield-check",
  },
  {
    slot: "database-config" as const,
    title: isZh.value ? "数据库 & 基础配置" : "Database & Basics",
    description: isZh.value ? "配置数据库、存储、缓存等基础服务" : "Configure database, storage, and cache",
    icon: "i-lucide-database",
  },
  {
    slot: "domain-https" as const,
    title: isZh.value ? "域名与 HTTPS" : "Domain & HTTPS",
    description: isZh.value ? "配置外部访问域名和 TLS 设置" : "Configure domain and TLS options",
    icon: "i-lucide-globe",
  },
  {
    slot: "admin-tenant" as const,
    title: isZh.value ? "超级管理员 & 租户初始化" : "Admin & Tenant Init",
    description: isZh.value ? "创建管理员账户和初始化租户" : "Create admin account and initialize tenant",
    icon: "i-lucide-user-cog",
  },
  {
    slot: "plugins-install" as const,
    title: isZh.value ? "插件与智能体安装" : "Plugins & Agents",
    description: isZh.value ? "安装基础插件和智能体组件" : "Install basic plugins and agent components",
    icon: "i-lucide-puzzle",
  },
  {
    slot: "llm-config" as const,
    title: isZh.value ? "LLM 模型配置" : "LLM Configuration",
    description: isZh.value ? "可选：配置文本大模型接入参数" : "Optional: configure text LLM access",
    icon: "i-lucide-bot",
  },
]);
const legalCopy = computed(() =>
  String(locale.value || "").startsWith("zh")
    ? {
        sectionTitle: "许可与条款",
        agreePrefix: "我已阅读并同意",
        license: "软件许可协议",
        terms: "服务条款",
        privacy: "隐私政策",
        and: "和",
        telemetryTitle: "启用匿名遥测数据收集（可选，默认关闭）",
        telemetryDesc: "帮助我们改进产品，不会收集敏感信息",
        licenseModalTitle: "软件许可协议",
        licenseLines: [
          "本软件按“现状”提供，仅用于合法业务场景。",
          "未经授权不得复制、反编译、转售或用于违法用途。",
          "生产环境请自行评估并承担部署、运维与数据合规责任。",
          "如需正式法务版本，请替换为贵司最终《软件许可协议》文本。",
        ],
      }
    : {
        sectionTitle: "License and Terms",
        agreePrefix: "I have read and agree to",
        license: "Software License Agreement",
        terms: "Terms of Service",
        privacy: "Privacy Policy",
        and: "and",
        telemetryTitle: "Enable anonymous telemetry (optional, disabled by default)",
        telemetryDesc: "Helps improve the product without collecting sensitive information.",
        licenseModalTitle: "Software License Agreement",
        licenseLines: [
          "This software is provided \"as is\" and must be used for lawful purposes only.",
          "Unauthorized copying, reverse engineering, resale, or illegal use is prohibited.",
          "In production, you are responsible for deployment, operations, and compliance.",
          "Replace this text with your organization's final legal license agreement if needed.",
        ],
      }
);

// 步骤数据
const step1Data = reactive({
  checks: {
    database: { status: "pending", message: "检测中..." },
    storage: { status: "pending", message: "检测中..." },
    cache: { status: "pending", message: "检测中..." },
    email: { status: "pending", message: "检测中..." },
    ai: { status: "pending", message: "检测中..." },
  },
  deploymentMode: "multi-tenant",
  authMode: "builtin",
  licenseAccepted: false,
  termsAccepted: false,
  telemetryEnabled: false,
});

const step2Data = reactive({
  domain: "",
  apiSubdomain: "",
  httpsMode: "auto",
  certEmail: "",
  certContent: "",
  keyContent: "",
  enableCdn: false,
  cdnDomain: "",
});

const step3Data = reactive({
  backendPort: 8080,
  webAdminPort: 3000,
  dbType: "postgresql",
  dbVersion: "",
  dbHost: "localhost",
  dbPort: 5432,
  dbName: "powerx",
  dbCharset: "utf8mb4",
  dbUsername: "root",
  dbPassword: "",
  sqlitePath: "/data/powerx.db",
  cacheType: "redis",
  redisHost: "localhost",
  redisPort: 6379,
  redisPassword: "",
  redisDb: 0,
  storageType: "local",
  localStoragePath: "/data/uploads",
  storageAccessKey: "",
  storageSecretKey: "",
  storageBucket: "",
  storageRegion: "",
  emailEnabled: false,
  emailSmtpHost: "",
  emailSmtpPort: 587,
  emailFromName: "",
  emailFromAddress: "",
});

const step4Data = reactive({
  adminUsername: "admin",
  adminEmail: "",
  adminPassword: "",
  adminPasswordConfirm: "",
  adminName: "",
  adminPhone: "",
  tenantName: "默认租户",
  tenantCode: "default",
  tenantDescription: "",
  createDefaultDepartments: true,
  companyName: "我的公司",
});

const step5Data = reactive({
  availablePlugins: [
    {
      id: "workflow",
      name: "审批流程",
      description: "提供工作流和审批功能",
      category: "workflow",
      recommended: true,
    },
    {
      id: "notification",
      name: "消息通知",
      description: "邮件、短信、站内信通知",
      category: "communication",
      recommended: true,
    },
    {
      id: "file-manager",
      name: "文件管理",
      description: "文件上传、管理和分享",
      category: "storage",
      recommended: true,
    },
    {
      id: "ai-assistant",
      name: "AI 助手",
      description: "智能对话和任务处理",
      category: "ai",
      recommended: false,
    },
  ],
});

const step6Data = reactive({
  skipLLMSetup: false,
  provider: "openai",
  baseUrl: "",
  model: "gpt-4.1-mini",
  apiKey: "",
  temperature: 0.7,
  topP: 1,
  maxTokens: 4096,
  stream: true,
});
const llmLastTestMessage = ref("");
const llmLastTestDetail = ref("");
const llmProviderOptions = ref<Array<{ label: string; value: string }>>([]);
const qwenFallbackModels = [
  "qwen-max-latest",
  "qwen-plus-latest",
  "qwen-turbo-latest",
  "qwen-flash-latest",
  "qwen-plus",
  "qwen-turbo",
  "qwen-max",
  "qwen3.5-plus",
  "qwen3.5-flash",
  "qwen3.5-plus-2026-02-15",
  "qwen3.5-flash-2026-02-23",
  "qwen3-max-latest",
  "qwen3-plus-latest",
  "qwen3-turbo-latest",
  "qwen3-flash-latest",
  "qwen3-max",
  "qwen3-plus",
  "qwen3-turbo",
  "qwen3-flash",
  "qwen3-coder-plus",
  "qwen3-coder-flash",
  "qwen2.5-72b-instruct",
  "qwen2.5-32b-instruct",
  "qwen2.5-14b-instruct",
  "qwen2.5-7b-instruct",
  "qwq-plus",
  "qwq-32b",
].map((m) => ({ label: m, value: m }));
const llmModelOptionsMap = ref<Record<string, Array<{ label: string; value: string }>>>({
  openai: [
    { label: "gpt-4.1-mini", value: "gpt-4.1-mini" },
    { label: "gpt-4o-mini", value: "gpt-4o-mini" },
    { label: "gpt-4.1", value: "gpt-4.1" },
  ],
  ollama: [
    { label: "llama3", value: "llama3" },
    { label: "qwen2.5:7b", value: "qwen2.5:7b" },
    { label: "deepseek-r1:7b", value: "deepseek-r1:7b" },
  ],
  openrouter: [
    { label: "openai/gpt-4.1-mini", value: "openai/gpt-4.1-mini" },
    { label: "anthropic/claude-3.5-sonnet", value: "anthropic/claude-3.5-sonnet" },
  ],
  huggingface: [
    { label: "Qwen/Qwen2.5-72B-Instruct", value: "Qwen/Qwen2.5-72B-Instruct" },
    { label: "meta-llama/Meta-Llama-3.1-8B-Instruct", value: "meta-llama/Meta-Llama-3.1-8B-Instruct" },
  ],
  moonshot: [
    { label: "moonshot-v1-8k", value: "moonshot-v1-8k" },
    { label: "moonshot-v1-32k", value: "moonshot-v1-32k" },
  ],
  volcengine: [
    { label: "doubao-seed-1-6", value: "doubao-seed-1-6" },
    { label: "doubao-1.5-pro-32k", value: "doubao-1.5-pro-32k" },
  ],
  hunyuan: [
    { label: "hunyuan-turbo", value: "hunyuan-turbo" },
    { label: "hunyuan-pro", value: "hunyuan-pro" },
  ],
  "qwen-intl": qwenFallbackModels,
  "qwen-cn": qwenFallbackModels,
  openai_compatible: [
    { label: "gpt-4o-mini", value: "gpt-4o-mini" },
    { label: "qwen-plus", value: "qwen-plus" },
  ],
  custom: [
    { label: "custom-model", value: "custom-model" },
  ],
});
const llmModelOptions = computed(() => {
  const p = String(step6Data.provider || "").trim();
  return llmModelOptionsMap.value[p] || llmModelOptionsMap.value.openai || [];
});

const buildFallbackLLMProviderOptions = () => [
  { label: "OpenAI", value: "openai" },
  { label: "Ollama (Local)", value: "ollama" },
  { label: "OpenRouter", value: "openrouter" },
  { label: "Hugging Face Inference", value: "huggingface" },
  { label: "Moonshot AI (Kimi)", value: "moonshot" },
  { label: "Volcengine (ByteDance)", value: "volcengine" },
  { label: "Tencent Hunyuan", value: "hunyuan" },
  { label: isZh.value ? "Qwen（官方国际）" : "Qwen (Intl)", value: "qwen-intl" },
  { label: isZh.value ? "Qwen（官方国内）" : "Qwen (CN)", value: "qwen-cn" },
  { label: "OpenAI Compatible", value: "openai_compatible" },
  { label: isZh.value ? "自定义" : "Custom", value: "custom" },
];

const normalizeProviderValue = (raw: unknown): string =>
  String(raw || "").trim().toLowerCase();

const isSetupInstalled = computed(() => {
  const status = String(
    setupStatus.value?.install?.status ??
      setupStatus.value?.install_status ??
      setupStatus.value?.status ??
      ""
  )
    .trim()
    .toLowerCase();
  return status === "installed";
});

const desiredBackendPort = computed(() =>
  Number(setupStatus.value?.desired_ports?.backend_port || step3Data.backendPort || 0)
);
const desiredWebAdminPort = computed(() =>
  Number(setupStatus.value?.desired_ports?.web_admin_port || step3Data.webAdminPort || 0)
);
const effectiveBackendPort = computed(() =>
  Number(setupStatus.value?.effective_ports?.backend_port || 0)
);
const effectiveWebAdminPort = computed(() =>
  Number(setupStatus.value?.effective_ports?.web_admin_port || 0)
);
const restartRequired = computed(() => Boolean(setupStatus.value?.restart_required));
const desiredSource = computed(() =>
  String(setupStatus.value?.config_source?.desired_ports || "")
);
const effectiveSource = computed(() =>
  String(setupStatus.value?.config_source?.effective_ports || "")
);

const syncProviderAndModelDefaults = () => {
  const firstProvider = llmProviderOptions.value[0]?.value || "openai";
  if (!llmProviderOptions.value.some((p) => p.value === step6Data.provider)) {
    step6Data.provider = firstProvider;
  }
  const firstModel = llmModelOptions.value[0]?.value || "";
  if (!llmModelOptions.value.some((m) => m.value === step6Data.model)) {
    step6Data.model = firstModel;
  }
};

const isValidHttpBaseURL = (raw: string): boolean => {
  const val = String(raw || "").trim();
  return /^https?:\/\//i.test(val);
};

const isLikelyHTMLPayload = (payload: unknown): boolean => {
  if (typeof payload !== "string") return false;
  const raw = payload.trim().toLowerCase();
  return raw.startsWith("<!doctype html") || raw.startsWith("<html");
};

const loadLLMProviderOptions = async () => {
  if (!isSetupInstalled.value) {
    llmProviderOptions.value = buildFallbackLLMProviderOptions();
    syncProviderAndModelDefaults();
    return;
  }
  try {
    const providers = await AISettingService.getProviders("llm", "dev");
    const items = (providers || [])
      .map((p: any) => ({
        label: String(p?.Name || p?.ID || "").trim(),
        value: normalizeProviderValue(p?.ID),
      }))
      .filter((p) => p.label && p.value);
    llmProviderOptions.value = items.length ? items : buildFallbackLLMProviderOptions();
  } catch {
    llmProviderOptions.value = buildFallbackLLMProviderOptions();
  }
  syncProviderAndModelDefaults();
};

const loadLLMModelsByProvider = async (provider: string) => {
  if (!isSetupInstalled.value) {
    return;
  }
  const p = normalizeProviderValue(provider);
  if (!p) return;
  try {
    const models = await AISettingService.getModels(p, "llm", "dev");
    const items = (models || [])
      .map((m) => String(m || "").trim())
      .filter((m) => m.length > 0)
      .map((m) => ({ label: m, value: m }));
    if (items.length) {
      llmModelOptionsMap.value = {
        ...llmModelOptionsMap.value,
        [p]: items,
      };
    }
  } catch {
    // setup 阶段容错：失败时保持静态兜底模型
  }
  syncProviderAndModelDefaults();
};

watch(
  () => step6Data.provider,
  async (provider, prevProvider) => {
    await loadLLMModelsByProvider(provider);
    // provider 切换后，模型选中值必须联动为该 provider 的首个可用模型
    if (prevProvider !== undefined && provider !== prevProvider) {
      step6Data.model = llmModelOptions.value[0]?.value || "";
    }
  },
  { immediate: true }
);

const setupTestConnection = async () => {
  llmLastTestMessage.value = "";
  llmLastTestDetail.value = "";
  const rawBaseURL = String(step6Data.baseUrl || "").trim();
  if (rawBaseURL && !isValidHttpBaseURL(rawBaseURL)) {
    llmLastTestMessage.value = isZh.value ? "连接测试失败。" : "Connection test failed.";
    llmLastTestDetail.value = isZh.value
      ? "Base URL 必须以 http:// 或 https:// 开头。"
      : "Base URL must start with http:// or https://.";
    return;
  }
  try {
    const resp: any = await $fetch("/api/v1/admin/setup/llm/test-connection", {
      method: "POST",
      body: {
        env: "dev",
        provider: step6Data.provider,
        model: step6Data.model,
        baseURL: step6Data.baseUrl,
        apiKey: step6Data.apiKey,
      },
    });
    if (isLikelyHTMLPayload(resp)) {
      throw new Error(
        isZh.value
          ? "连接测试返回了 HTML 页面而非 JSON，请检查前端 API 代理是否正确。"
          : "Connection test returned HTML instead of JSON. Please check frontend API proxy settings."
      );
    }
    const data = resp?.data ?? resp;
    if (isLikelyHTMLPayload(data)) {
      throw new Error(
        isZh.value
          ? "连接测试返回了 HTML 页面而非 JSON，请检查前端 API 代理是否正确。"
          : "Connection test returned HTML instead of JSON. Please check frontend API proxy settings."
      );
    }
    if (!data || typeof data !== "object" || (Object.prototype.hasOwnProperty.call(data, "ok") && data.ok !== true)) {
      throw new Error(
        isZh.value
          ? "连接测试响应格式异常（期望 JSON 对象）。"
          : "Unexpected connection test response format (expected JSON object)."
      );
    }
    llmLastTestMessage.value = isZh.value
      ? "连接测试通过。"
      : "Connection test passed.";
    llmLastTestDetail.value = JSON.stringify(data, null, 2);
  } catch (error: any) {
    llmLastTestMessage.value = isZh.value
      ? "连接测试失败。"
      : "Connection test failed.";
    llmLastTestDetail.value = String(error?.data?.error || error?.data?.message || error?.message || "unknown error");
  }
};

const setupTestQuickCall = async () => {
  llmLastTestMessage.value = "";
  llmLastTestDetail.value = "";
  const rawBaseURL = String(step6Data.baseUrl || "").trim();
  if (rawBaseURL && !isValidHttpBaseURL(rawBaseURL)) {
    llmLastTestMessage.value = isZh.value ? "试跑失败。" : "Quick run failed.";
    llmLastTestDetail.value = isZh.value
      ? "Base URL 必须以 http:// 或 https:// 开头。"
      : "Base URL must start with http:// or https://.";
    return;
  }
  try {
    const resp: any = await $fetch("/api/v1/admin/setup/llm/test-call", {
      method: "POST",
      body: {
        env: "dev",
        provider: step6Data.provider,
        model: step6Data.model,
        baseURL: step6Data.baseUrl,
        apiKey: step6Data.apiKey,
        temperature: Number(step6Data.temperature || 0),
        maxTokens: Number(step6Data.maxTokens || 0),
        prompt: "hello",
      },
    });
    if (isLikelyHTMLPayload(resp)) {
      throw new Error(
        isZh.value
          ? "试跑返回了 HTML 页面而非 JSON，请检查前端 API 代理是否正确。"
          : "Quick run returned HTML instead of JSON. Please check frontend API proxy settings."
      );
    }
    const data = resp?.data ?? resp;
    if (isLikelyHTMLPayload(data) || !data || typeof data !== "object") {
      throw new Error(
        isZh.value
          ? "试跑响应格式异常（期望 JSON 对象）。"
          : "Unexpected quick run response format (expected JSON object)."
      );
    }
    llmLastTestMessage.value = String(data?.text || data?.output || (isZh.value ? "试跑成功。" : "Quick run succeeded."));
    llmLastTestDetail.value = JSON.stringify(data, null, 2);
  } catch (error: any) {
    llmLastTestMessage.value = isZh.value
      ? "试跑失败。"
      : "Quick run failed.";
    llmLastTestDetail.value = String(error?.data?.error || error?.data?.message || error?.message || "unknown error");
  }
};

const dbTestState = reactive({
  database: {
    testing: false,
    ok: false,
    message: "",
  },
  cache: {
    testing: false,
    ok: false,
    message: "",
  },
  provisioning: false,
  provisioned: false,
});

const setupPayload = computed(() => ({
  domain: {
    domain: step2Data.domain,
    api_subdomain: step2Data.apiSubdomain,
    enable_cdn: step2Data.enableCdn,
    cdn_domain: step2Data.cdnDomain,
  },
  https: {
    mode: step2Data.httpsMode as "auto" | "manual" | "disable",
    cert_email: step2Data.certEmail,
    cert_content: step2Data.certContent,
    key_content: step2Data.keyContent,
  },
  storage: {
    type: normalizeStorageType(step3Data.storageType),
    local_path: step3Data.localStoragePath,
    access_key: step3Data.storageAccessKey,
    secret_key: step3Data.storageSecretKey,
    bucket: step3Data.storageBucket,
    region: step3Data.storageRegion,
  },
  cache: {
    type: step3Data.cacheType,
    redis_host: step3Data.redisHost,
    redis_port: Number(step3Data.redisPort || 0),
    redis_db: Number(step3Data.redisDb || 0),
    redis_password: step3Data.redisPassword,
  },
  email: {
    enabled: step3Data.emailEnabled,
    smtp_host: step3Data.emailSmtpHost,
    smtp_port: Number(step3Data.emailSmtpPort || 0),
    from_name: step3Data.emailFromName,
    from_address: step3Data.emailFromAddress,
  },
  llm: {
    enabled: !step6Data.skipLLMSetup,
    provider: step6Data.provider,
    model: step6Data.model,
    base_url: step6Data.baseUrl,
    api_key: step6Data.apiKey,
    temperature: Number(step6Data.temperature || 0),
    top_p: Number(step6Data.topP || 0),
    max_tokens: Number(step6Data.maxTokens || 0),
    stream: !!step6Data.stream,
  },
  database: {
    type: step3Data.dbType,
    host: step3Data.dbHost,
    port: Number(step3Data.dbPort || 0),
    name: step3Data.dbName,
    username: step3Data.dbUsername,
    password: step3Data.dbPassword,
    charset: step3Data.dbCharset,
    ssl_mode: "disable",
    sqlite_path: step3Data.sqlitePath,
  },
  ports: {
    backend_port: Number(step3Data.backendPort || 0),
    web_admin_port: Number(step3Data.webAdminPort || 0),
  },
}));

const setupTestDatabaseConnection = async () => {
  dbTestState.database.testing = true;
  dbTestState.database.message = "";
  try {
    const resp: any = await $fetch("/api/v1/admin/setup/test/database", {
      method: "POST",
      body: {
        database: setupPayload.value.database,
      },
    });
    const data = resp?.data ?? resp;
    if (!data || data.ok !== true) {
      throw new Error(isZh.value ? "数据库连接测试返回异常" : "Database test response is invalid");
    }
    dbTestState.database.ok = true;
    dbTestState.database.message = isZh.value ? "数据库连接成功" : "Database connection is healthy";
  } catch (error: any) {
    dbTestState.database.ok = false;
    dbTestState.database.message = String(error?.data?.error || error?.data?.message || error?.message || "unknown error");
  } finally {
    dbTestState.database.testing = false;
  }
};

const setupTestCacheConnection = async () => {
  dbTestState.cache.testing = true;
  dbTestState.cache.message = "";
  try {
    const resp: any = await $fetch("/api/v1/admin/setup/test/cache", {
      method: "POST",
      body: {
        cache: setupPayload.value.cache,
      },
    });
    const data = resp?.data ?? resp;
    if (!data || data.ok !== true) {
      throw new Error(isZh.value ? "缓存连接测试返回异常" : "Cache test response is invalid");
    }
    dbTestState.cache.ok = true;
    dbTestState.cache.message = String(data?.message || (isZh.value ? "缓存连接成功" : "Cache connection is healthy"));
  } catch (error: any) {
    dbTestState.cache.ok = false;
    dbTestState.cache.message = String(error?.data?.error || error?.data?.message || error?.message || "unknown error");
  } finally {
    dbTestState.cache.testing = false;
  }
};

const provisionSetupAtDatabaseStep = async (): Promise<boolean> => {
  if (dbTestState.provisioned) {
    return true;
  }
  dbTestState.provisioning = true;
  try {
    await settingsService.saveSetupConfig(setupPayload.value as any);
    const resp: any = await $fetch("/api/v1/admin/setup/provision", {
      method: "POST",
    });
    const data = resp?.data ?? resp;
    if (!data || data.ok !== true) {
      throw new Error(isZh.value ? "数据库初始化响应异常" : "Provision response is invalid");
    }
    dbTestState.provisioned = true;
    toast.add({
      title: isZh.value ? "数据库初始化完成" : "Database provision completed",
      description: isZh.value ? "已执行 migrate/seed，可继续下一步。" : "migrate/seed finished. You can continue.",
      color: "success",
    });
    return true;
  } catch (error: any) {
    dbTestState.provisioned = false;
    toast.add({
      title: isZh.value ? "数据库初始化失败" : "Database provision failed",
      description: String(error?.data?.error || error?.data?.message || error?.message || "unknown error"),
      color: "error",
    });
    return false;
  } finally {
    dbTestState.provisioning = false;
  }
};

watch(
  () => [
    step3Data.dbType,
    step3Data.dbHost,
    step3Data.dbPort,
    step3Data.dbName,
    step3Data.dbUsername,
    step3Data.dbPassword,
    step3Data.sqlitePath,
    step3Data.cacheType,
    step3Data.redisHost,
    step3Data.redisPort,
    step3Data.redisDb,
    step3Data.redisPassword,
  ],
  () => {
    dbTestState.provisioned = false;
  }
);

// 验证当前步骤
const validateCurrentStep = () => {
  switch (currentStep.value) {
    case 0:
      return step1Data.licenseAccepted && step1Data.termsAccepted;
    case 1:
      return step3Data.dbType === "sqlite" || step3Data.dbHost.trim() !== "";
    case 2:
      return (
        step2Data.domain.trim() !== "" &&
        Number(step3Data.backendPort) > 0 &&
        Number(step3Data.backendPort) <= 65535 &&
        Number(step3Data.webAdminPort) > 0 &&
        Number(step3Data.webAdminPort) <= 65535 &&
        Number(step3Data.backendPort) !== Number(step3Data.webAdminPort)
      );
    case 3:
      if (Number(setupStatus.value?.checks?.users || 0) > 0) {
        return true;
      }
      return (
        step4Data.adminUsername &&
        step4Data.adminEmail &&
        step4Data.adminPassword &&
        step4Data.adminPassword === step4Data.adminPasswordConfirm
      );
    case 4:
      return true;
    case 5:
      if (step6Data.skipLLMSetup) return true;
      return (
        String(step6Data.provider || "").trim() !== "" &&
        String(step6Data.model || "").trim() !== "" &&
        String(step6Data.apiKey || "").trim() !== ""
      );
    default:
      return false;
  }
};

// 下一步
const nextStep = async () => {
  if (!validateCurrentStep()) return;

  isLoading.value = true;
  try {
    if (currentStep.value === 1) {
      const ok = await provisionSetupAtDatabaseStep();
      if (!ok) return;
    }
    if (currentStep.value < stepperItems.value.length - 1) {
      currentStep.value++;
    } else {
      await completeSetup();
    }
  } finally {
    isLoading.value = false;
  }
};

// 上一步
const prevStep = () => {
  if (currentStep.value > 0) {
    currentStep.value--;
  }
};

// 完成设置
const completeSetup = async () => {
  completing.value = true;
  try {
    await settingsService.saveSetupConfig(setupPayload.value as any);

    await $fetch("/api/v1/admin/setup/complete", {
      method: "POST",
      body: {
        licenseAccepted: step1Data.licenseAccepted,
        termsAccepted: step1Data.termsAccepted,
      },
    });
    await navigateTo("/home");
  } catch (error: any) {
    toast.add({
      title: "保存失败",
      description: String(error?.data?.message || error?.message || "安装配置保存失败"),
      color: "error",
    });
  } finally {
    completing.value = false;
  }
};

// 执行系统检测
const runSystemChecks = async () => {
  const checks = ["database", "storage", "cache", "email", "ai"];
  for (const check of checks) {
    step1Data.checks[check].status = "checking";
    step1Data.checks[check].message = uiCopy.value.checking;
  }

  try {
    const resp: any = await settingsService.getSetupStatus();
    const payload = resp?.data ?? resp;
    setupStatus.value = payload;

    const hasTenant = Number(payload?.checks?.tenants || 0) > 0;
    const hasUser = Number(payload?.checks?.users || 0) > 0;
    const hasAI = Number(payload?.checks?.ai_profiles || 0) > 0;

    if (isSetupInstalled.value) {
      loadLLMProviderOptions();
    }

    step1Data.checks.database.status = hasTenant ? "success" : "pending";
    step1Data.checks.database.message = hasTenant ? uiCopy.value.msgTenantInited : uiCopy.value.msgTenantNotInited;
    step1Data.checks.storage.status = "pending";
    step1Data.checks.storage.message = uiCopy.value.msgStoragePending;
    step1Data.checks.cache.status = "pending";
    step1Data.checks.cache.message = uiCopy.value.msgCachePending;
    step1Data.checks.email.status = "pending";
    step1Data.checks.email.message = uiCopy.value.msgEmailPending;
    step1Data.checks.ai.status = hasAI ? "success" : "pending";
    step1Data.checks.ai.message = hasAI ? uiCopy.value.msgAIOk : uiCopy.value.msgAINone;

    if (hasUser) {
      step4Data.adminUsername = "";
      step4Data.adminEmail = "";
    }
  } catch {
    for (const check of checks) {
      step1Data.checks[check].status = "error";
      step1Data.checks[check].message = uiCopy.value.msgCheckFailed;
    }
  }
};

const normalizeStorageType = (raw: string): string => {
  const value = String(raw || "").trim().toLowerCase();
  if (value === "aliyun") return "oss";
  if (value === "tencent") return "cos";
  if (value === "aws") return "s3";
  return value || "local";
};

const denormalizeStorageType = (raw: string): string => {
  const value = String(raw || "").trim().toLowerCase();
  if (value === "oss") return "aliyun";
  if (value === "cos") return "tencent";
  if (value === "s3") return "aws";
  return value || "local";
};

const loadSetupConfig = async () => {
  try {
    const resp: any = await settingsService.getSetupConfig();
    const payload = resp?.data ?? resp;
    const cfg = payload?.config;
    if (!cfg) return;

    step2Data.domain = String(cfg.domain?.domain || step2Data.domain);
    step2Data.apiSubdomain = String(cfg.domain?.api_subdomain || step2Data.apiSubdomain);
    step2Data.enableCdn = Boolean(cfg.domain?.enable_cdn);
    step2Data.cdnDomain = String(cfg.domain?.cdn_domain || step2Data.cdnDomain);

    step2Data.httpsMode = String(cfg.https?.mode || step2Data.httpsMode);
    step2Data.certEmail = String(cfg.https?.cert_email || step2Data.certEmail);
    step2Data.certContent = String(cfg.https?.cert_content || step2Data.certContent);
    step2Data.keyContent = String(cfg.https?.key_content || step2Data.keyContent);

    step3Data.storageType = denormalizeStorageType(cfg.storage?.type);
    step3Data.localStoragePath = String(cfg.storage?.local_path || step3Data.localStoragePath);
    step3Data.storageAccessKey = String(cfg.storage?.access_key || step3Data.storageAccessKey);
    step3Data.storageSecretKey = String(cfg.storage?.secret_key || step3Data.storageSecretKey);
    step3Data.storageBucket = String(cfg.storage?.bucket || step3Data.storageBucket);
    step3Data.storageRegion = String(cfg.storage?.region || step3Data.storageRegion);

    step3Data.cacheType = String(cfg.cache?.type || step3Data.cacheType);
    step3Data.redisHost = String(cfg.cache?.redis_host || step3Data.redisHost);
    step3Data.redisPort = Number(cfg.cache?.redis_port || step3Data.redisPort);
    step3Data.redisDb = Number(cfg.cache?.redis_db || step3Data.redisDb);
    step3Data.redisPassword = String(cfg.cache?.redis_password || step3Data.redisPassword);

    step3Data.dbType = String(cfg.database?.type || step3Data.dbType);
    step3Data.dbHost = String(cfg.database?.host || step3Data.dbHost);
    step3Data.dbPort = Number(cfg.database?.port || step3Data.dbPort);
    step3Data.dbName = String(cfg.database?.name || step3Data.dbName);
    step3Data.dbUsername = String(cfg.database?.username || step3Data.dbUsername);
    step3Data.dbPassword = String(cfg.database?.password || step3Data.dbPassword);
    step3Data.dbCharset = String(cfg.database?.charset || step3Data.dbCharset);
    step3Data.sqlitePath = String(cfg.database?.sqlite_path || step3Data.sqlitePath);

    const llmEnabled = cfg.llm?.enabled !== false;
    step6Data.skipLLMSetup = !llmEnabled;
    step6Data.provider = String(cfg.llm?.provider || step6Data.provider);
    step6Data.model = String(cfg.llm?.model || step6Data.model);
    step6Data.baseUrl = String(cfg.llm?.base_url || step6Data.baseUrl);
    step6Data.apiKey = String(cfg.llm?.api_key || step6Data.apiKey);
    step6Data.temperature = Number(cfg.llm?.temperature ?? step6Data.temperature);
    step6Data.topP = Number(cfg.llm?.top_p ?? step6Data.topP);
    step6Data.maxTokens = Number(cfg.llm?.max_tokens ?? step6Data.maxTokens);
    step6Data.stream = Boolean(cfg.llm?.stream ?? step6Data.stream);

    step3Data.emailEnabled = Boolean(cfg.email?.enabled);
    step3Data.emailSmtpHost = String(cfg.email?.smtp_host || step3Data.emailSmtpHost);
    step3Data.emailSmtpPort = Number(cfg.email?.smtp_port || step3Data.emailSmtpPort);
    step3Data.emailFromName = String(cfg.email?.from_name || step3Data.emailFromName);
    step3Data.emailFromAddress = String(cfg.email?.from_address || step3Data.emailFromAddress);

    step3Data.backendPort = Number(cfg.ports?.backend_port || step3Data.backendPort);
    step3Data.webAdminPort = Number(cfg.ports?.web_admin_port || step3Data.webAdminPort);
  } catch {
    // 首次没有配置时忽略
  }
};

const openLicenseModal = (e: Event) => {
  e.preventDefault();
  showLicenseModal.value = true;
};

const openTermsModal = (e: Event) => {
  e.preventDefault();
  showTermsModal.value = true;
};

const openPrivacyModal = (e: Event) => {
  e.preventDefault();
  showPrivacyModal.value = true;
};

onMounted(() => {
  llmProviderOptions.value = buildFallbackLLMProviderOptions();
  loadLLMProviderOptions();
  loadSetupConfig();
  runSystemChecks();
});
</script>

<template>
  <div
    class="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-gray-800"
  >
    <div class="container mx-auto px-4 py-8">
      <div class="flex justify-end items-center gap-2 mb-4">
        <LanguageSwitcher />
        <ThemeSwitcher />
      </div>

      <!-- 页面标题 -->
      <div class="text-center mb-8">
        <h1 class="text-3xl font-bold text-gray-900 dark:text-white mb-2">
          {{ uiCopy.pageTitle }}
        </h1>
        <p class="text-gray-600 dark:text-gray-300">
          {{ uiCopy.pageDesc }}
        </p>
      </div>

      <!-- 步骤指示器和内容 -->
      <div class="max-w-4xl mx-auto">
        <UStepper
          ref="stepper"
          :items="stepperItems"
          v-model="currentStep"
          class="w-full"
        >
          <!-- 步骤1：系统检查 & 许可 -->
          <template #system-check>
            <UCard class="shadow-lg mt-6">
              <div class="p-6">
                <h3 class="text-lg font-semibold mb-6">{{ uiCopy.step1Title }}</h3>
                <p v-if="setupStatus?.version" class="text-sm text-gray-500 mb-4">
                  {{ uiCopy.versionLabel }}{{ setupStatus.version }}
                </p>

                <!-- 系统检测 -->
                <div class="mb-8">
                  <h4 class="text-md font-medium mb-4">{{ uiCopy.envCheckTitle }}</h4>
                  <div class="space-y-3">
                    <div
                      v-for="(check, key) in step1Data.checks"
                      :key="key"
                      class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700 rounded-lg"
                    >
                      <div class="flex items-center space-x-3">
                        <UIcon
                          :name="
                            check.status === 'success'
                              ? 'i-lucide-check-circle'
                              : check.status === 'error'
                                ? 'i-lucide-x-circle'
                                : 'i-lucide-clock'
                          "
                          :class="{
                            'text-green-500': check.status === 'success',
                            'text-red-500': check.status === 'error',
                            'text-yellow-500': check.status === 'checking',
                            'text-gray-400': check.status === 'pending',
                          }"
                          class="w-5 h-5"
                        />
                        <span class="font-medium">
                          {{ uiCopy.checkLabelMap[String(key)] || String(key) }}
                        </span>
                      </div>
                      <span
                        class="text-sm"
                        :class="{
                          'text-green-600': check.status === 'success',
                          'text-red-600': check.status === 'error',
                          'text-yellow-600': check.status === 'checking',
                          'text-gray-500': check.status === 'pending',
                        }"
                      >
                        {{ check.message }}
                      </span>
                    </div>
                  </div>
                </div>

                <!-- 部署模式选择 -->
                <div class="mb-8">
                  <h4 class="text-md font-medium mb-4">部署模式</h4>
                  <div class="grid grid-cols-2 gap-4 mb-4">
                    <UCard
                      :class="{
                        'ring-2 ring-blue-500':
                          step1Data.deploymentMode === 'single-tenant',
                        'hover:shadow-md cursor-pointer': true,
                      }"
                      @click="step1Data.deploymentMode = 'single-tenant'"
                    >
                      <div class="p-4">
                        <div class="flex items-center space-x-3">
                          <input
                            v-model="step1Data.deploymentMode"
                            type="radio"
                            value="single-tenant"
                          />
                          <div>
                            <h5 class="font-medium">单租户模式</h5>
                            <p class="text-sm text-gray-600 dark:text-gray-300">
                              适合单一组织使用
                            </p>
                          </div>
                        </div>
                      </div>
                    </UCard>

                    <UCard
                      :class="{
                        'ring-2 ring-blue-500':
                          step1Data.deploymentMode === 'multi-tenant',
                        'hover:shadow-md cursor-pointer': true,
                      }"
                      @click="step1Data.deploymentMode = 'multi-tenant'"
                    >
                      <div class="p-4">
                        <div class="flex items-center space-x-3">
                          <input
                            v-model="step1Data.deploymentMode"
                            type="radio"
                            value="multi-tenant"
                          />
                          <div>
                            <h5 class="font-medium">多租户模式</h5>
                            <p class="text-sm text-gray-600 dark:text-gray-300">
                              支持多个组织独立使用
                            </p>
                          </div>
                        </div>
                      </div>
                    </UCard>
                  </div>

                  <h5 class="font-medium mb-3">接入方式</h5>
                  <div class="grid grid-cols-2 gap-4">
                    <UCard
                      :class="{
                        'ring-2 ring-blue-500':
                          step1Data.authMode === 'builtin',
                        'hover:shadow-md cursor-pointer': true,
                      }"
                      @click="step1Data.authMode = 'builtin'"
                    >
                      <div class="p-4">
                        <div class="flex items-center space-x-3">
                          <input
                            v-model="step1Data.authMode"
                            type="radio"
                            value="builtin"
                          />
                          <div>
                            <h5 class="font-medium">内置账户</h5>
                            <p class="text-sm text-gray-600 dark:text-gray-300">
                              使用系统内置的用户管理
                            </p>
                          </div>
                        </div>
                      </div>
                    </UCard>

                    <UCard
                      :class="{
                        'ring-2 ring-blue-500': step1Data.authMode === 'sso',
                        'hover:shadow-md cursor-pointer': true,
                      }"
                      @click="step1Data.authMode = 'sso'"
                    >
                      <div class="p-4">
                        <div class="flex items-center space-x-3">
                          <input v-model="step1Data.authMode" type="radio" value="sso" />
                          <div>
                            <h5 class="font-medium">SSO 单点登录</h5>
                            <p class="text-sm text-gray-600 dark:text-gray-300">
                              集成企业身份认证系统
                            </p>
                          </div>
                        </div>
                      </div>
                    </UCard>
                  </div>
                </div>

                <!-- 许可确认 -->
                <div class="space-y-4">
                  <h4 class="text-md font-medium">{{ legalCopy.sectionTitle }}</h4>

                  <div class="flex items-start space-x-3">
                    <UCheckbox
                      :model-value="step1Data.licenseAccepted"
                      @update:model-value="(v) => (step1Data.licenseAccepted = !!v)"
                      class="mt-1"
                    />
                    <div>
                      <p class="text-sm">
                        {{ legalCopy.agreePrefix }}
                        <button
                          type="button"
                          class="text-blue-600 hover:underline"
                          @click="openLicenseModal"
                        >
                          {{ legalCopy.license }}
                        </button>
                      </p>
                    </div>
                  </div>

                  <div class="flex items-start space-x-3">
                    <UCheckbox
                      :model-value="step1Data.termsAccepted"
                      @update:model-value="(v) => (step1Data.termsAccepted = !!v)"
                      class="mt-1"
                    />
                    <div>
                      <p class="text-sm">
                        {{ legalCopy.agreePrefix }}
                        <button
                          type="button"
                          class="text-blue-600 hover:underline"
                          @click="openTermsModal"
                        >
                          {{ legalCopy.terms }}
                        </button>
                        {{ legalCopy.and }}
                        <button
                          type="button"
                          class="text-blue-600 hover:underline"
                          @click="openPrivacyModal"
                        >
                          {{ legalCopy.privacy }}
                        </button>
                      </p>
                    </div>
                  </div>

                  <div class="flex items-start space-x-3">
                    <UCheckbox
                      :model-value="step1Data.telemetryEnabled"
                      @update:model-value="(v) => (step1Data.telemetryEnabled = !!v)"
                      class="mt-1"
                    />
                    <div>
                      <p class="text-sm">
                        {{ legalCopy.telemetryTitle }}
                      </p>
                      <p class="text-xs text-gray-500 mt-1">
                        {{ legalCopy.telemetryDesc }}
                      </p>
                    </div>
                  </div>
                </div>
              </div>

              <!-- 底部按钮 -->
              <template #footer>
                <div class="flex justify-between items-center px-6 py-4">
                  <div></div>
                  <UButton
                    color="primary"
                    @click="nextStep"
                    :loading="isLoading"
                    :disabled="!validateCurrentStep()"
                  >
                    下一步
                  </UButton>
                </div>
              </template>
            </UCard>
          </template>

          <!-- 步骤2：域名 & HTTPS -->
          <template #domain-https>
            <UCard class="shadow-lg mt-6">
              <div class="p-6">
                <h3 class="text-lg font-semibold mb-6">域名 & HTTPS 配置</h3>

                <!-- 域名配置 -->
                <div class="mb-8">
                  <h4 class="text-md font-medium mb-4">域名设置</h4>
                  <div class="space-y-4">
                    <UFormField label="系统域名" required>
                      <UInput
                        v-model="step2Data.domain"
                        placeholder="例如：powerx.example.com"
                        icon="i-lucide-globe"
                      />
                      <template #help>
                        <span class="text-sm text-gray-500">
                          请输入系统的完整域名，不包含协议前缀
                        </span>
                      </template>
                    </UFormField>

                    <UFormField label="API 子域名">
                      <UInput
                        v-model="step2Data.apiSubdomain"
                        placeholder="例如：api"
                        icon="i-lucide-server"
                      />
                      <template #help>
                        <span class="text-sm text-gray-500">
                          API 接口的子域名，留空则使用主域名
                        </span>
                      </template>
                    </UFormField>
                  </div>
                </div>

                <!-- HTTPS 配置 -->
                <div class="mb-8">
                  <h4 class="text-md font-medium mb-4">HTTPS 配置</h4>
                  <div class="space-y-4">
                    <URadioGroup
                      v-model="step2Data.httpsMode"
                      :options="[
                        {
                          value: 'auto',
                          label: '自动获取证书 (Let\'s Encrypt)',
                        },
                        { value: 'manual', label: '手动上传证书' },
                        { value: 'disable', label: '暂不启用 HTTPS' },
                      ]"
                    />

                    <!-- 自动证书配置 -->
                    <div
                      v-if="step2Data.httpsMode === 'auto'"
                      class="pl-6 space-y-4"
                    >
                      <UFormField label="邮箱地址" required>
                        <UInput
                          v-model="step2Data.certEmail"
                          type="email"
                          placeholder="admin@example.com"
                          icon="i-lucide-mail"
                        />
                        <template #help>
                          <span class="text-sm text-gray-500">
                            用于 Let's Encrypt 证书申请和续期通知
                          </span>
                        </template>
                      </UFormField>
                    </div>

                    <!-- 手动证书上传 -->
                    <div
                      v-if="step2Data.httpsMode === 'manual'"
                      class="pl-6 space-y-4"
                    >
                      <UFormField label="证书文件 (.crt)" required>
                        <UTextarea
                          v-model="step2Data.certContent"
                          placeholder="-----BEGIN CERTIFICATE-----"
                          rows="6"
                        />
                      </UFormField>

                      <UFormField label="私钥文件 (.key)" required>
                        <UTextarea
                          v-model="step2Data.keyContent"
                          placeholder="-----BEGIN PRIVATE KEY-----"
                          rows="6"
                        />
                      </UFormField>
                    </div>

                    <!-- HTTPS 禁用警告 -->
                    <UAlert
                      v-if="step2Data.httpsMode === 'disable'"
                      icon="i-lucide-alert-triangle"
                      color="amber"
                      variant="soft"
                      title="安全警告"
                      description="不启用 HTTPS 可能存在安全风险，建议在生产环境中启用"
                    />
                  </div>
                </div>

                <!-- 服务端口 -->
                <div class="mb-8">
                  <h4 class="text-md font-medium mb-4">服务端口</h4>
                  <div class="grid grid-cols-2 gap-4">
                    <UFormField label="Backend 端口" required>
                      <UInput
                        v-model="step3Data.backendPort"
                        type="number"
                        placeholder="8080"
                      />
                    </UFormField>

                    <UFormField label="Web Admin 端口" required>
                      <UInput
                        v-model="step3Data.webAdminPort"
                        type="number"
                        placeholder="3000"
                      />
                    </UFormField>
                  </div>
                  <div class="mt-4 rounded-lg border border-gray-200 dark:border-gray-700">
                    <div class="grid grid-cols-3 gap-2 bg-gray-50 dark:bg-gray-800 px-3 py-2 text-xs font-semibold">
                      <span>{{ isZh ? "项目" : "Item" }}</span>
                      <span>{{ isZh ? "目标值（Desired）" : "Desired" }}</span>
                      <span>{{ isZh ? "当前生效值（Effective）" : "Effective" }}</span>
                    </div>
                    <div class="grid grid-cols-3 gap-2 px-3 py-2 text-xs border-t border-gray-100 dark:border-gray-700">
                      <span>Backend</span>
                      <span>{{ desiredBackendPort || "-" }}</span>
                      <span>{{ effectiveBackendPort || "-" }}</span>
                    </div>
                    <div class="grid grid-cols-3 gap-2 px-3 py-2 text-xs border-t border-gray-100 dark:border-gray-700">
                      <span>Web Admin</span>
                      <span>{{ desiredWebAdminPort || "-" }}</span>
                      <span>{{ effectiveWebAdminPort || "-" }}</span>
                    </div>
                  </div>
                  <UAlert
                    v-if="restartRequired"
                    class="mt-3"
                    color="warning"
                    variant="soft"
                    icon="i-lucide-refresh-cw"
                    :title="isZh ? '端口配置已变更，需重启服务后生效' : 'Port config changed. Restart services to take effect.'"
                    :description="isZh
                      ? `desired=${desiredSource || '-'}; effective=${effectiveSource || '-'}`
                      : `desired=${desiredSource || '-'}; effective=${effectiveSource || '-'}`"
                  />
                  <p class="text-xs text-gray-500 mt-2">
                    推荐生产默认：backend=8080 / web-admin=3000；开发口径：
                    backend=8077 / web-admin=3030
                  </p>
                </div>

                <!-- CDN 配置 -->
                <div class="space-y-4">
                  <h4 class="text-md font-medium mb-4">CDN 配置（可选）</h4>
                  <UCheckbox
                    :model-value="step2Data.enableCdn"
                    @update:model-value="(v) => (step2Data.enableCdn = !!v)"
                    label="启用 CDN 加速"
                  />

                  <div v-if="step2Data.enableCdn" class="pl-6 space-y-4">
                    <UFormField label="CDN 域名">
                      <UInput
                        v-model="step2Data.cdnDomain"
                        placeholder="例如：cdn.example.com"
                        icon="i-lucide-zap"
                      />
                    </UFormField>
                  </div>
                </div>
              </div>

              <template #footer>
                <div class="flex justify-between items-center px-6 py-4">
                  <UButton
                    variant="outline"
                    @click="prevStep"
                    :disabled="isLoading"
                  >
                    上一步
                  </UButton>
                  <UButton
                    color="primary"
                    @click="nextStep"
                    :loading="isLoading"
                    :disabled="!validateCurrentStep()"
                  >
                    下一步
                  </UButton>
                </div>
              </template>
            </UCard>
          </template>

          <!-- 步骤3：数据库 & 基础配置 -->
          <template #database-config>
            <UCard class="shadow-lg mt-6">
              <div class="p-6">
                <h3 class="text-lg font-semibold mb-6">数据库 & 基础配置</h3>

                <!-- 数据库配置 -->
                <div class="mb-8">
                  <h4 class="text-md font-medium mb-4">数据库设置</h4>
                  <div class="grid grid-cols-2 gap-4 mb-4">
                    <UFormField label="数据库类型" required>
                      <USelect
                        v-model="step3Data.dbType"
                        :portal="false"
                        :options="[
                          { label: 'MySQL', value: 'mysql' },
                          { label: 'PostgreSQL', value: 'postgresql' },
                          { label: 'SQLite', value: 'sqlite' },
                        ]"
                      />
                    </UFormField>

                    <UFormField label="数据库版本">
                      <UInput
                        v-model="step3Data.dbVersion"
                        placeholder="例如：8.0"
                        :disabled="step3Data.dbType === 'sqlite'"
                      />
                    </UFormField>
                  </div>

                  <div
                    v-if="step3Data.dbType !== 'sqlite'"
                    class="grid grid-cols-2 gap-4 mb-4"
                  >
                    <UFormField label="主机地址" required>
                      <UInput
                        v-model="step3Data.dbHost"
                        placeholder="localhost"
                        icon="i-lucide-server"
                      />
                    </UFormField>

                    <UFormField label="端口" required>
                      <UInput
                        v-model="step3Data.dbPort"
                        type="number"
                        :placeholder="
                          step3Data.dbType === 'mysql' ? '3306' : '5432'
                        "
                      />
                    </UFormField>
                  </div>

                  <div
                    v-if="step3Data.dbType !== 'sqlite'"
                    class="grid grid-cols-2 gap-4 mb-4"
                  >
                    <UFormField label="数据库名" required>
                      <UInput
                        v-model="step3Data.dbName"
                        placeholder="powerx"
                        icon="i-lucide-database"
                      />
                    </UFormField>

                    <UFormField label="字符集">
                      <USelect
                        v-model="step3Data.dbCharset"
                        :portal="false"
                        :options="[
                          { label: 'utf8mb4', value: 'utf8mb4' },
                          { label: 'utf8', value: 'utf8' },
                        ]"
                      />
                    </UFormField>
                  </div>

                  <template v-if="step3Data.dbType !== 'sqlite'">
                    <div class="grid grid-cols-2 gap-4">
                      <UFormField label="用户名" required>
                        <UInput
                          v-model="step3Data.dbUsername"
                          placeholder="root"
                          icon="i-lucide-user"
                        />
                      </UFormField>

                      <UFormField label="密码" required>
                        <UInput
                          v-model="step3Data.dbPassword"
                          type="password"
                          placeholder="数据库密码"
                          icon="i-lucide-lock"
                        />
                      </UFormField>
                    </div>

                    <div class="mt-4 grid grid-cols-2 gap-4 items-center">
                      <p
                        class="text-xs"
                        :class="dbTestState.database.ok ? 'text-green-500' : 'text-gray-500'"
                      >
                        {{ dbTestState.database.message || '可先测试数据库连通性，再进入下一步。' }}
                      </p>
                      <div class="flex justify-start">
                        <UButton
                          color="neutral"
                          variant="soft"
                          :loading="dbTestState.database.testing"
                          @click="setupTestDatabaseConnection"
                        >
                          测试数据库连接
                        </UButton>
                      </div>
                    </div>
                  </template>

                  <div v-else class="space-y-4">
                    <UFormField label="数据库文件路径">
                      <UInput
                        v-model="step3Data.sqlitePath"
                        placeholder="/data/powerx.db"
                        icon="i-lucide-file"
                      />
                    </UFormField>
                  </div>
                </div>

                <!-- 缓存配置 -->
                <div class="mb-8">
                  <h4 class="text-md font-medium mb-4">缓存配置</h4>
                  <div class="space-y-4">
                    <UFormField label="缓存类型">
                      <USelect
                        v-model="step3Data.cacheType"
                        :portal="false"
                        :options="[
                          { label: 'Redis', value: 'redis' },
                          { label: 'Memcached', value: 'memcached' },
                          { label: '文件缓存', value: 'file' },
                        ]"
                      />
                    </UFormField>

                    <div
                      v-if="step3Data.cacheType === 'redis'"
                      class="grid grid-cols-2 gap-4"
                    >
                      <UFormField label="Redis 主机">
                        <UInput
                          v-model="step3Data.redisHost"
                          placeholder="localhost"
                          icon="i-lucide-server"
                        />
                      </UFormField>

                      <UFormField label="Redis 端口">
                        <UInput
                          v-model="step3Data.redisPort"
                          type="number"
                          placeholder="6379"
                        />
                      </UFormField>
                    </div>

                    <div
                      v-if="step3Data.cacheType === 'redis'"
                      class="grid grid-cols-2 gap-4"
                    >
                      <UFormField label="Redis 密码">
                        <UInput
                          v-model="step3Data.redisPassword"
                          type="password"
                          placeholder="Redis 密码（可选）"
                          icon="i-lucide-lock"
                        />
                      </UFormField>

                      <UFormField label="数据库索引">
                        <UInput
                          v-model="step3Data.redisDb"
                          type="number"
                          placeholder="0"
                        />
                      </UFormField>
                    </div>

                    <div
                      v-if="step3Data.cacheType === 'redis'"
                      class="mt-2 grid grid-cols-2 gap-4 items-center"
                    >
                      <p
                        class="text-xs"
                        :class="dbTestState.cache.ok ? 'text-green-500' : 'text-gray-500'"
                      >
                        {{ dbTestState.cache.message || '建议测试 Redis 连通性，避免初始化阶段失败。' }}
                      </p>
                      <div class="flex justify-start">
                        <UButton
                          color="neutral"
                          variant="soft"
                          :loading="dbTestState.cache.testing"
                          @click="setupTestCacheConnection"
                        >
                          测试 Redis 连接
                        </UButton>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 存储配置 -->
                <div class="space-y-4">
                  <h4 class="text-md font-medium mb-4">文件存储配置</h4>
                  <UFormField label="存储类型">
                    <USelect
                      v-model="step3Data.storageType"
                      :portal="false"
                      :options="[
                        { label: '本地存储', value: 'local' },
                        { label: '阿里云 OSS', value: 'aliyun' },
                        { label: '腾讯云 COS', value: 'tencent' },
                        { label: 'AWS S3', value: 'aws' },
                      ]"
                    />
                  </UFormField>

                  <div
                    v-if="step3Data.storageType === 'local'"
                    class="space-y-4"
                  >
                    <UFormField label="存储路径">
                      <UInput
                        v-model="step3Data.localStoragePath"
                        placeholder="/data/uploads"
                        icon="i-lucide-folder"
                      />
                    </UFormField>
                  </div>

                  <div
                    v-if="step3Data.storageType !== 'local'"
                    class="grid grid-cols-2 gap-4"
                  >
                    <UFormField label="Access Key ID" required>
                      <UInput
                        v-model="step3Data.storageAccessKey"
                        placeholder="访问密钥 ID"
                        icon="i-lucide-key"
                      />
                    </UFormField>

                    <UFormField label="Secret Access Key" required>
                      <UInput
                        v-model="step3Data.storageSecretKey"
                        type="password"
                        placeholder="访问密钥"
                        icon="i-lucide-lock"
                      />
                    </UFormField>
                  </div>

                  <div
                    v-if="step3Data.storageType !== 'local'"
                    class="grid grid-cols-2 gap-4"
                  >
                    <UFormField label="存储桶名称" required>
                      <UInput
                        v-model="step3Data.storageBucket"
                        placeholder="bucket-name"
                        icon="i-lucide-archive"
                      />
                    </UFormField>

                    <UFormField label="区域" required>
                      <UInput
                        v-model="step3Data.storageRegion"
                        placeholder="例如：cn-hangzhou"
                        icon="i-lucide-map-pin"
                      />
                    </UFormField>
                  </div>
                </div>

                <!-- 邮件配置 -->
                <div class="mt-8 space-y-4">
                  <h4 class="text-md font-medium mb-4">邮件配置（可选）</h4>

                  <UFormField label="启用 SMTP 邮件">
                    <USwitch v-model="step3Data.emailEnabled" />
                  </UFormField>

                  <div v-if="step3Data.emailEnabled" class="grid grid-cols-2 gap-4">
                    <UFormField label="SMTP 主机" required>
                      <UInput
                        v-model="step3Data.emailSmtpHost"
                        placeholder="smtp.example.com"
                        icon="i-lucide-mail"
                      />
                    </UFormField>
                    <UFormField label="SMTP 端口" required>
                      <UInput
                        v-model="step3Data.emailSmtpPort"
                        type="number"
                        placeholder="587"
                      />
                    </UFormField>
                  </div>

                  <div v-if="step3Data.emailEnabled" class="grid grid-cols-2 gap-4">
                    <UFormField label="发件人名称">
                      <UInput
                        v-model="step3Data.emailFromName"
                        placeholder="PowerX"
                      />
                    </UFormField>
                    <UFormField label="发件人邮箱" required>
                      <UInput
                        v-model="step3Data.emailFromAddress"
                        placeholder="noreply@example.com"
                      />
                    </UFormField>
                  </div>
                </div>
              </div>

              <template #footer>
                <div class="flex justify-between items-center px-6 py-4">
                  <UButton
                    variant="outline"
                    @click="prevStep"
                    :disabled="isLoading"
                  >
                    上一步
                  </UButton>
                  <div class="flex items-center gap-3">
                    <p
                      class="text-xs"
                      :class="dbTestState.provisioned ? 'text-green-500' : 'text-gray-500'"
                    >
                      {{
                        dbTestState.provisioned
                          ? '数据库 migrate/seed 已完成'
                          : '下一步会自动保存配置并执行数据库初始化'
                      }}
                    </p>
                    <UButton
                      color="primary"
                      @click="nextStep"
                      :loading="isLoading || dbTestState.provisioning"
                      :disabled="!validateCurrentStep()"
                    >
                      下一步
                    </UButton>
                  </div>
                </div>
              </template>
            </UCard>
          </template>

          <!-- 步骤4：超级管理员 & 租户初始化 -->
          <template #admin-tenant>
            <UCard class="shadow-lg mt-6">
              <div class="p-6">
                <h3 class="text-lg font-semibold mb-6">
                  超级管理员 & 租户初始化
                </h3>

                <!-- 超级管理员配置 -->
                <div class="mb-8">
                  <h4 class="text-md font-medium mb-4">超级管理员账户</h4>
                  <div class="grid grid-cols-2 gap-4 mb-4">
                    <UFormField label="管理员用户名" required>
                      <UInput
                        v-model="step4Data.adminUsername"
                        placeholder="admin"
                        icon="i-lucide-user"
                      />
                    </UFormField>

                    <UFormField label="管理员邮箱" required>
                      <UInput
                        v-model="step4Data.adminEmail"
                        type="email"
                        placeholder="admin@example.com"
                        icon="i-lucide-mail"
                      />
                    </UFormField>
                  </div>

                  <div class="grid grid-cols-2 gap-4 mb-4">
                    <UFormField label="管理员密码" required>
                      <UInput
                        v-model="step4Data.adminPassword"
                        type="password"
                        placeholder="请输入强密码"
                        icon="i-lucide-lock"
                      />
                    </UFormField>

                    <UFormField label="确认密码" required>
                      <UInput
                        v-model="step4Data.adminPasswordConfirm"
                        type="password"
                        placeholder="再次输入密码"
                        icon="i-lucide-lock"
                      />
                    </UFormField>
                  </div>

                  <div class="grid grid-cols-2 gap-4">
                    <UFormField label="管理员姓名">
                      <UInput
                        v-model="step4Data.adminName"
                        placeholder="系统管理员"
                        icon="i-lucide-user-circle"
                      />
                    </UFormField>

                    <UFormField label="手机号码">
                      <UInput
                        v-model="step4Data.adminPhone"
                        placeholder="13800138000"
                        icon="i-lucide-phone"
                      />
                    </UFormField>
                  </div>
                </div>

                <!-- 默认租户配置 -->
                <div
                  class="mb-8"
                  v-if="step1Data.deploymentMode === 'multi-tenant'"
                >
                  <h4 class="text-md font-medium mb-4">默认租户设置</h4>
                  <div class="grid grid-cols-2 gap-4 mb-4">
                    <UFormField label="租户名称" required>
                      <UInput
                        v-model="step4Data.tenantName"
                        placeholder="默认租户"
                        icon="i-lucide-building"
                      />
                    </UFormField>

                    <UFormField label="租户标识" required>
                      <UInput
                        v-model="step4Data.tenantCode"
                        placeholder="default"
                        icon="i-lucide-tag"
                      />
                      <template #help>
                        <span class="text-sm text-gray-500">
                          租户的唯一标识，只能包含字母、数字和下划线
                        </span>
                      </template>
                    </UFormField>
                  </div>

                  <div class="space-y-4">
                    <UFormField label="租户描述">
                      <UTextarea
                        v-model="step4Data.tenantDescription"
                        placeholder="租户描述信息"
                        :rows="3"
                      />
                    </UFormField>
                  </div>
                </div>

                <!-- 组织架构初始化 -->
                <div class="space-y-4">
                  <h4 class="text-md font-medium mb-4">组织架构初始化</h4>

                  <UCheckbox
                    :model-value="step4Data.createDefaultDepartments"
                    @update:model-value="(v) => (step4Data.createDefaultDepartments = !!v)"
                    label="创建默认部门结构"
                  />

                  <div
                    v-if="step4Data.createDefaultDepartments"
                    class="pl-6 space-y-4"
                  >
                    <div class="grid grid-cols-2 gap-4">
                      <UFormField label="公司名称">
                        <UInput
                          v-model="step4Data.companyName"
                          placeholder="我的公司"
                          icon="i-lucide-building-2"
                        />
                      </UFormField>
                    </div>
                  </div>
                </div>
              </div>

              <template #footer>
                <div class="flex justify-between items-center px-6 py-4">
                  <UButton
                    variant="outline"
                    @click="prevStep"
                    :disabled="isLoading"
                  >
                    上一步
                  </UButton>
                  <UButton
                    color="primary"
                    @click="nextStep"
                    :loading="isLoading"
                    :disabled="!validateCurrentStep()"
                  >
                    下一步
                  </UButton>
                </div>
              </template>
            </UCard>
          </template>

          <!-- 步骤5：插件与智能体安装 -->
          <template #plugins-install>
            <UCard class="shadow-lg mt-6">
              <div class="p-6">
                <h3 class="text-lg font-semibold mb-6">插件与智能体安装</h3>

                <div class="mb-6">
                  <p class="text-gray-600 dark:text-gray-300 text-sm">
                    以下为安装后建议启用的基础能力清单（引导说明，不会在此步骤立即执行插件安装）。
                  </p>
                </div>

                <div class="space-y-4">
                  <div
                    v-for="plugin in step5Data.availablePlugins"
                    :key="plugin.id"
                    class="flex items-center justify-between p-4 border rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700"
                  >
                    <div>
                      <h5 class="font-medium">{{ plugin.name }}</h5>
                      <p class="text-sm text-gray-600 dark:text-gray-300">
                        {{ plugin.description }}
                      </p>
                    </div>
                    <div class="flex items-center space-x-2">
                      <span
                        class="px-2 py-1 text-xs rounded-full"
                        :class="{
                          'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200':
                            plugin.category === 'workflow',
                          'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200':
                            plugin.category === 'communication',
                          'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200':
                            plugin.category === 'storage',
                          'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200':
                            plugin.category === 'ai',
                        }"
                      >
                        {{
                          plugin.category === "workflow"
                            ? "工作流"
                            : plugin.category === "communication"
                              ? "通信"
                              : plugin.category === "storage"
                                ? "存储"
                                : plugin.category === "ai"
                                  ? "AI"
                                  : plugin.category
                        }}
                      </span>
                      <span
                        v-if="plugin.recommended"
                        class="px-2 py-1 text-xs bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200 rounded-full"
                      >
                        推荐
                      </span>
                    </div>
                  </div>
                </div>

                <div class="mt-6 p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
                  <div class="flex items-start space-x-3">
                    <UIcon
                      name="i-lucide-info"
                      class="w-5 h-5 text-blue-500 mt-0.5"
                    />
                    <div>
                      <h5 class="font-medium text-blue-900 dark:text-blue-100">
                        提示
                      </h5>
                      <p class="text-sm text-blue-700 dark:text-blue-300 mt-1">
                        本步骤仅提供建议清单。实际安装与启停请在“插件市场/已安装插件”中操作，再在“模型中心”“工具中心”完成配置与密钥绑定。
                      </p>
                    </div>
                  </div>
                </div>
              </div>

              <template #footer>
                <div class="flex justify-between items-center px-6 py-4">
                  <UButton
                    variant="outline"
                    @click="prevStep"
                    :disabled="isLoading"
                  >
                    上一步
                  </UButton>
                  <UButton
                    color="primary"
                    @click="nextStep"
                    :loading="isLoading"
                  >
                    下一步
                  </UButton>
                </div>
              </template>
            </UCard>
          </template>

          <!-- 步骤6：LLM 配置（可选） -->
          <template #llm-config>
            <UCard class="shadow-lg mt-6">
              <div class="p-6">
                <h3 class="text-lg font-semibold mb-2">
                  {{ isZh ? "LLM 模型配置（可选）" : "LLM Configuration (Optional)" }}
                </h3>
                <p class="text-sm text-gray-500 mb-6">
                  {{
                    isZh
                      ? "仅配置文本模型接入参数；图像/音频等其他模态可在安装后继续配置。"
                      : "Configure text LLM only. Other modalities can be configured after installation."
                  }}
                </p>

                <div class="space-y-4">
                  <UCheckbox
                    :model-value="step6Data.skipLLMSetup"
                    @update:model-value="(v) => (step6Data.skipLLMSetup = !!v)"
                    :label="isZh ? '跳过此步骤，安装后在模型中心配置' : 'Skip this step and configure later in Model Center'"
                  />
                </div>

                <div v-if="!step6Data.skipLLMSetup" class="grid grid-cols-1 lg:grid-cols-3 gap-6 mt-6">
                  <div class="lg:col-span-2 space-y-6">
                    <div class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4">
                      <div class="mb-4 font-medium text-[var(--text-primary)]">
                        {{ isZh ? "LLM 文本 - 通用" : "LLM Text - General" }}
                      </div>
                      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                        <UFormField class="md:col-span-2" :label="isZh ? 'Provider' : 'Provider'" required>
                          <USelect
                            v-model="step6Data.provider"
                            :items="llmProviderOptions"
                            icon="i-heroicons-building-library"
                            class="w-full"
                          />
                        </UFormField>
                        <UFormField class="md:col-span-2" :label="isZh ? 'Model' : 'Model'" required>
                          <USelect
                            v-model="step6Data.model"
                            :items="llmModelOptions"
                            icon="i-heroicons-cpu-chip"
                            class="w-full"
                          />
                          <p class="mt-1 text-xs text-[var(--text-secondary)] break-all leading-5">
                            {{ step6Data.model }}
                          </p>
                        </UFormField>
                        <UFormField class="md:col-span-2 w-full" :label="isZh ? 'Base URL（可选）' : 'Base URL (Optional)'">
                          <UInput
                            v-model="step6Data.baseUrl"
                            :placeholder="isZh ? '例如：https://api.openai.com/v1 或 http://127.0.0.1:11434/v1' : 'e.g. https://api.openai.com/v1 or http://127.0.0.1:11434/v1'"
                            class="w-full"
                          />
                        </UFormField>
                        <UFormField class="md:col-span-2 w-full" :label="isZh ? 'API Key' : 'API Key'" required>
                          <UInput
                            v-model="step6Data.apiKey"
                            type="password"
                            autocomplete="off"
                            :placeholder="isZh ? '请输入 API Key' : 'Enter API Key'"
                            class="w-full"
                          />
                        </UFormField>
                      </div>
                    </div>

                    <div class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4">
                      <div class="mb-4 font-medium text-[var(--text-primary)]">
                        {{ isZh ? "LLM 文本 - 参数" : "LLM Text - Parameters" }}
                      </div>
                      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                        <div>
                          <label class="block text-sm font-medium text-[var(--text-primary)] mb-2">
                            {{ isZh ? "温度 (Temperature)" : "Temperature" }}
                          </label>
                          <USlider v-model="step6Data.temperature" :min="0" :max="2" :step="0.1" class="w-full" />
                          <div class="text-xs text-[var(--text-secondary)] mt-1">
                            {{ isZh ? "当前值" : "Current" }}: {{ step6Data.temperature }}
                          </div>
                        </div>
                        <div>
                          <label class="block text-sm font-medium text-[var(--text-primary)] mb-2">
                            {{ isZh ? "最大令牌数" : "Max Tokens" }}
                          </label>
                          <UInput v-model="step6Data.maxTokens" type="number" :min="1" :max="32000" />
                        </div>
                        <div>
                          <label class="block text-sm font-medium text-[var(--text-primary)] mb-2">
                            Top P
                          </label>
                          <USlider v-model="step6Data.topP" :min="0" :max="1" :step="0.01" class="w-full" />
                          <div class="text-xs text-[var(--text-secondary)] mt-1">
                            {{ isZh ? "当前值" : "Current" }}: {{ step6Data.topP }}
                          </div>
                        </div>
                        <div class="flex items-center">
                          <UCheckbox
                            :model-value="step6Data.stream"
                            @update:model-value="(v) => (step6Data.stream = !!v)"
                            :label="isZh ? '启用流式输出' : 'Enable Streaming'"
                          />
                        </div>
                      </div>
                    </div>
                  </div>

                  <div class="lg:col-span-1">
                    <TestPanel
                      :current-title="isZh ? 'LLM 文本' : 'LLM Text'"
                      :current-state="{
                        provider: step6Data.provider,
                        model: step6Data.model,
                        apiKey: step6Data.apiKey,
                        baseURL: step6Data.baseUrl,
                      }"
                      :last-test-message="llmLastTestMessage"
                      :last-test-detail="llmLastTestDetail"
                      :on-test-connection="setupTestConnection"
                      :on-test-quick-call="setupTestQuickCall"
                    />
                  </div>
                </div>
              </div>

              <template #footer>
                <div class="flex justify-between items-center px-6 py-4">
                  <UButton
                    variant="outline"
                    @click="prevStep"
                    :disabled="isLoading"
                  >
                    {{ isZh ? "上一步" : "Previous" }}
                  </UButton>
                  <UButton
                    color="primary"
                    @click="nextStep"
                    :loading="isLoading"
                    :disabled="!validateCurrentStep()"
                  >
                    {{ isZh ? "完成设置" : "Finish Setup" }}
                  </UButton>
                </div>
              </template>
            </UCard>
          </template>
        </UStepper>
      </div>
    </div>

    <UModal
      v-model:open="showLicenseModal"
      :title="legalCopy.licenseModalTitle"
      :ui="{ content: 'max-w-3xl max-h-[85dvh] overflow-y-auto' }"
    >
      <template #body>
        <div class="space-y-3 text-sm leading-6 text-gray-700 dark:text-gray-300">
          <p v-for="line in legalCopy.licenseLines" :key="line">{{ line }}</p>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="showTermsModal"
      :title="legalCopy.terms"
      :ui="{ content: 'max-w-3xl max-h-[85dvh] overflow-y-auto' }"
    >
      <template #body>
        <UsersTermsModal />
      </template>
    </UModal>

    <UModal
      v-model:open="showPrivacyModal"
      :title="legalCopy.privacy"
      :ui="{ content: 'max-w-3xl max-h-[85dvh] overflow-y-auto' }"
    >
      <template #body>
        <UsersPrivacyModal />
      </template>
    </UModal>
  </div>
</template>
