<script setup lang="ts">
import type { StepperItem } from "@nuxt/ui";

definePageMeta({
  layout: false,
});

// 系统初始化步骤 - 使用 Nuxt UI Stepper 格式
const stepperItems = [
  {
    slot: "system-check" as const,
    title: "系统检查 & 许可",
    description: "检测系统环境和确认使用条款",
    icon: "i-lucide-shield-check",
  },
  {
    slot: "domain-https" as const,
    title: "域名与 HTTPS",
    description: "配置外部访问域名和 TLS 设置",
    icon: "i-lucide-globe",
  },
  {
    slot: "database-config" as const,
    title: "数据库 & 基础配置",
    description: "配置数据库、存储、缓存等基础服务",
    icon: "i-lucide-database",
  },
  {
    slot: "admin-tenant" as const,
    title: "超级管理员 & 租户初始化",
    description: "创建管理员账户和初始化租户",
    icon: "i-lucide-user-cog",
  },
  {
    slot: "plugins-install" as const,
    title: "插件与智能体安装",
    description: "安装基础插件和智能体组件",
    icon: "i-lucide-puzzle",
  },
] satisfies StepperItem[];

// Stepper 引用和当前步骤
const stepper = ref();
const currentStep = ref(0);
const isLoading = ref(false);

// 步骤数据
const step1Data = reactive({
  checks: {
    database: { status: "pending", message: "检测中..." },
    storage: { status: "pending", message: "检测中..." },
    cache: { status: "pending", message: "检测中..." },
    email: { status: "pending", message: "检测中..." },
    sms: { status: "pending", message: "检测中..." },
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
      enabled: true,
    },
    {
      id: "notification",
      name: "消息通知",
      description: "邮件、短信、站内信通知",
      category: "communication",
      enabled: true,
    },
    {
      id: "file-manager",
      name: "文件管理",
      description: "文件上传、管理和分享",
      category: "storage",
      enabled: true,
    },
    {
      id: "ai-assistant",
      name: "AI 助手",
      description: "智能对话和任务处理",
      category: "ai",
      enabled: false,
    },
  ],
  selectedPlugins: ["workflow", "notification", "file-manager"],
});

// 验证当前步骤
const validateCurrentStep = () => {
  switch (currentStep.value) {
    case 0:
      return step1Data.licenseAccepted && step1Data.termsAccepted;
    case 1:
      return step2Data.domain.trim() !== "";
    case 2:
      return step3Data.dbHost.trim() !== "";
    case 3:
      return (
        step4Data.adminUsername &&
        step4Data.adminEmail &&
        step4Data.adminPassword &&
        step4Data.adminPassword === step4Data.adminPasswordConfirm
      );
    case 4:
      return true;
    default:
      return false;
  }
};

// 下一步
const nextStep = async () => {
  if (!validateCurrentStep()) return;

  isLoading.value = true;
  try {
    if (currentStep.value < stepperItems.length - 1) {
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
  setTimeout(() => {
    navigateTo("/dashboard");
  }, 2000);
};

// 执行系统检测
const runSystemChecks = async () => {
  const checks = ["database", "storage", "cache", "email", "sms"];
  for (const check of checks) {
    step1Data.checks[check].status = "checking";
    await new Promise((resolve) => setTimeout(resolve, 1000));
    const success = Math.random() > 0.2;
    step1Data.checks[check].status = success ? "success" : "error";
    step1Data.checks[check].message = success ? "连接正常" : "连接失败";
  }
};

onMounted(() => {
  runSystemChecks();
});
</script>

<template>
  <div
    class="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-gray-800"
  >
    <div class="container mx-auto px-4 py-8">
      <!-- 页面标题 -->
      <div class="text-center mb-8">
        <h1 class="text-3xl font-bold text-gray-900 dark:text-white mb-2">
          PowerX 系统初始化
        </h1>
        <p class="text-gray-600 dark:text-gray-300">
          欢迎使用 PowerX！请按照以下步骤完成系统初始化设置
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
                <h3 class="text-lg font-semibold mb-6">系统检查 & 许可</h3>

                <!-- 系统检测 -->
                <div class="mb-8">
                  <h4 class="text-md font-medium mb-4">系统环境检测</h4>
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
                          {{
                            key === "database"
                              ? "数据库连通性"
                              : key === "storage"
                                ? "对象存储"
                                : key === "cache"
                                  ? "缓存服务"
                                  : key === "email"
                                    ? "邮件服务"
                                    : key === "sms"
                                      ? "短信服务"
                                      : key
                          }}
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
                          <URadio
                            v-model="step1Data.deploymentMode"
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
                          <URadio
                            v-model="step1Data.deploymentMode"
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
                          <URadio
                            v-model="step1Data.authMode"
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
                          <URadio v-model="step1Data.authMode" value="sso" />
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
                  <h4 class="text-md font-medium">许可与条款</h4>

                  <div class="flex items-start space-x-3">
                    <UCheckbox
                      v-model="step1Data.licenseAccepted"
                      class="mt-1"
                    />
                    <div>
                      <p class="text-sm">
                        我已阅读并同意
                        <a href="#" class="text-blue-600 hover:underline"
                          >软件许可协议</a
                        >
                      </p>
                    </div>
                  </div>

                  <div class="flex items-start space-x-3">
                    <UCheckbox v-model="step1Data.termsAccepted" class="mt-1" />
                    <div>
                      <p class="text-sm">
                        我已阅读并同意
                        <a href="#" class="text-blue-600 hover:underline"
                          >服务条款</a
                        >
                        和
                        <a href="#" class="text-blue-600 hover:underline"
                          >隐私政策</a
                        >
                      </p>
                    </div>
                  </div>

                  <div class="flex items-start space-x-3">
                    <UCheckbox
                      v-model="step1Data.telemetryEnabled"
                      class="mt-1"
                    />
                    <div>
                      <p class="text-sm">
                        启用匿名遥测数据收集（可选，默认关闭）
                      </p>
                      <p class="text-xs text-gray-500 mt-1">
                        帮助我们改进产品，不会收集敏感信息
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

                <!-- CDN 配置 -->
                <div class="space-y-4">
                  <h4 class="text-md font-medium mb-4">CDN 配置（可选）</h4>
                  <UCheckbox
                    v-model="step2Data.enableCdn"
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
                        :options="[
                          { label: 'utf8mb4', value: 'utf8mb4' },
                          { label: 'utf8', value: 'utf8' },
                        ]"
                      />
                    </UFormField>
                  </div>

                  <div
                    v-if="step3Data.dbType !== 'sqlite'"
                    class="grid grid-cols-2 gap-4"
                  >
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
                  </div>
                </div>

                <!-- 存储配置 -->
                <div class="space-y-4">
                  <h4 class="text-md font-medium mb-4">文件存储配置</h4>
                  <UFormField label="存储类型">
                    <USelect
                      v-model="step3Data.storageType"
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
                        rows="3"
                      />
                    </UFormField>
                  </div>
                </div>

                <!-- 组织架构初始化 -->
                <div class="space-y-4">
                  <h4 class="text-md font-medium mb-4">组织架构初始化</h4>

                  <UCheckbox
                    v-model="step4Data.createDefaultDepartments"
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
                    选择要安装的基础插件。您可以在系统设置中随时添加或移除插件。
                  </p>
                </div>

                <div class="space-y-4">
                  <div
                    v-for="plugin in step5Data.availablePlugins"
                    :key="plugin.id"
                    class="flex items-center justify-between p-4 border rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700"
                  >
                    <div class="flex items-center space-x-4">
                      <UCheckbox
                        :model-value="
                          step5Data.selectedPlugins.includes(plugin.id)
                        "
                        @update:model-value="
                          (checked) => {
                            if (checked) {
                              if (
                                !step5Data.selectedPlugins.includes(plugin.id)
                              ) {
                                step5Data.selectedPlugins.push(plugin.id);
                              }
                            } else {
                              const index = step5Data.selectedPlugins.indexOf(
                                plugin.id
                              );
                              if (index > -1) {
                                step5Data.selectedPlugins.splice(index, 1);
                              }
                            }
                          }
                        "
                      />
                      <div>
                        <h5 class="font-medium">{{ plugin.name }}</h5>
                        <p class="text-sm text-gray-600 dark:text-gray-300">
                          {{ plugin.description }}
                        </p>
                      </div>
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
                        v-if="plugin.enabled"
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
                        插件安装完成后，您需要在"模型中心"、"工具中心"或"插件设置"中完成相关配置和密钥绑定。
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
                    完成设置
                  </UButton>
                </div>
              </template>
            </UCard>
          </template>
        </UStepper>
      </div>
    </div>
  </div>
</template>
