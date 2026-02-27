<script setup lang="ts">
import type { FormSubmitEvent } from "#ui/types";

definePageMeta({
  title: "系统配置",
  icon: "i-heroicons-wrench-screwdriver",
  order: 10,
});

const { t } = useI18n();

// 配置分类
interface ConfigCategory {
  id: string;
  name: string;
  icon: string;
  description: string;
}

const categories: ConfigCategory[] = [
  {
    id: "basic",
    name: "基础设置",
    icon: "i-heroicons-cog-6-tooth",
    description: "系统基本信息和通用配置",
  },
  {
    id: "security",
    name: "安全设置",
    icon: "i-heroicons-shield-check",
    description: "密码策略、登录限制等安全配置",
  },
  {
    id: "notification",
    name: "通知设置",
    icon: "i-heroicons-bell",
    description: "邮件、短信、系统通知配置",
  },
  {
    id: "storage",
    name: "存储设置",
    icon: "i-heroicons-server",
    description: "文件存储、数据库、缓存配置",
  },
  {
    id: "integration",
    name: "集成设置",
    icon: "i-heroicons-puzzle-piece",
    description: "第三方服务集成配置",
  },
  {
    id: "advanced",
    name: "高级设置",
    icon: "i-heroicons-wrench-screwdriver",
    description: "系统性能、日志、调试等高级配置",
  },
];

// 当前选中的分类
const selectedCategory = ref("basic");

// 基础设置表单
const basicForm = reactive({
  siteName: "PowerX Admin",
  siteDescription: "强大的企业级管理系统",
  siteUrl: "https://powerx.example.com",
  adminEmail: "admin@example.com",
  timezone: "Asia/Shanghai",
  language: "zh-CN",
  dateFormat: "YYYY-MM-DD",
  timeFormat: "24h",
});

// 安全设置表单
const securityForm = reactive({
  passwordMinLength: 8,
  passwordRequireUppercase: true,
  passwordRequireLowercase: true,
  passwordRequireNumbers: true,
  passwordRequireSymbols: false,
  loginMaxAttempts: 5,
  loginLockoutDuration: 30,
  sessionTimeout: 120,
  enableTwoFactor: false,
  enableCaptcha: true,
});

// 通知设置表单
const notificationForm = reactive({
  emailEnabled: true,
  emailHost: "smtp.example.com",
  emailPort: 587,
  emailUsername: "",
  emailPassword: "",
  emailFromName: "PowerX Admin",
  emailFromAddress: "noreply@example.com",
  smsEnabled: false,
  smsProvider: "aliyun",
  smsAccessKey: "",
  smsSecretKey: "",
  systemNotificationEnabled: true,
  browserNotificationEnabled: true,
});

// 存储设置表单
const storageForm = reactive({
  uploadMaxSize: 10,
  uploadAllowedTypes: ["jpg", "jpeg", "png", "gif", "pdf", "doc", "docx"],
  storageDriver: "local",
  storageEndpoint: "",
  storageAccessKey: "",
  storageSecretKey: "",
  storageBucket: "",
  cacheDriver: "redis",
  cacheHost: "localhost",
  cachePort: 6379,
  cachePassword: "",
});

// 集成设置表单
const integrationForm = reactive({
  wechatEnabled: false,
  wechatAppId: "",
  wechatAppSecret: "",
  alipayEnabled: false,
  alipayAppId: "",
  alipayPrivateKey: "",
  alipayPublicKey: "",
  analyticsEnabled: false,
  analyticsTrackingId: "",
  mapProvider: "baidu",
  mapApiKey: "",
});

// 高级设置表单
const advancedForm = reactive({
  debugMode: false,
  logLevel: "info",
  logMaxFiles: 30,
  cacheEnabled: true,
  compressionEnabled: true,
  maintenanceMode: false,
  maintenanceMessage: "系统维护中，请稍后访问",
  performanceMonitoring: true,
  errorReporting: true,
  backupEnabled: true,
  backupFrequency: "daily",
});

// 时区选项
const timezoneOptions = [
  { label: "北京时间 (UTC+8)", value: "Asia/Shanghai" },
  { label: "东京时间 (UTC+9)", value: "Asia/Tokyo" },
  { label: "纽约时间 (UTC-5)", value: "America/New_York" },
  { label: "伦敦时间 (UTC+0)", value: "Europe/London" },
];

// 语言选项
const languageOptions = [
  { label: "简体中文", value: "zh-CN" },
  { label: "English", value: "en-US" },
  { label: "日本語", value: "ja-JP" },
  { label: "한국어", value: "ko-KR" },
];

// 缓存驱动选项
const cacheDriverOptions = [
  { label: "Redis", value: "redis" },
  { label: "Memcached", value: "memcached" },
  { label: "文件缓存", value: "file" },
];

// 存储驱动选项
const storageDriverOptions = [
  { label: "本地存储", value: "local" },
  { label: "阿里云OSS", value: "oss" },
  { label: "腾讯云COS", value: "cos" },
  { label: "AWS S3", value: "s3" },
];

// 日志级别选项
const logLevelOptions = [
  { label: "Debug", value: "debug" },
  { label: "Info", value: "info" },
  { label: "Warning", value: "warning" },
  { label: "Error", value: "error" },
];

// 备份频率选项
const backupFrequencyOptions = [
  { label: "每日", value: "daily" },
  { label: "每周", value: "weekly" },
  { label: "每月", value: "monthly" },
];

// 保存配置
const saveConfig = async (category: string) => {
  try {
    // 这里应该调用API保存配置
    console.log(`保存${category}配置`);

    // 显示成功提示
    const toast = useToast();
    toast.add({
      title: "保存成功",
      description: "配置已成功保存",
      color: "success",
    });
  } catch (error) {
    console.error("保存配置失败:", error);
    const toast = useToast();
    toast.add({
      title: "保存失败",
      description: "配置保存失败，请重试",
      color: "error",
    });
  }
};

// 重置配置
const resetConfig = (category: string) => {
  // 这里应该重置对应分类的配置到默认值
  console.log(`重置${category}配置`);
};

// 测试连接（用于邮件、短信等配置）
const testConnection = async (type: string) => {
  try {
    // 这里应该调用API测试连接
    console.log(`测试${type}连接`);

    const toast = useToast();
    toast.add({
      title: "测试成功",
      description: `${type}连接测试成功`,
      color: "success",
    });
  } catch (error) {
    console.error(`${type}连接测试失败:`, error);
    const toast = useToast();
    toast.add({
      title: "测试失败",
      description: `${type}连接测试失败`,
      color: "error",
    });
  }
};
</script>

<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900">
        {{ t("menu.systemConfig") }}
      </h1>
      <p class="text-gray-600 mt-2">系统参数和配置管理</p>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-4 gap-6">
      <!-- 配置分类侧边栏 -->
      <div class="lg:col-span-1">
        <UCard>
          <template #header>
            <h3 class="text-lg font-semibold">配置分类</h3>
          </template>

          <div class="space-y-2">
            <button
              v-for="category in categories"
              :key="category.id"
              @click="selectedCategory = category.id"
              :class="[
                'w-full text-left p-3 rounded-lg transition-colors',
                selectedCategory === category.id
                  ? 'bg-primary-50 text-primary-700 border border-primary-200'
                  : 'hover:bg-gray-50',
              ]"
            >
              <div class="flex items-center space-x-3">
                <UIcon :name="category.icon" class="w-5 h-5" />
                <div>
                  <div class="font-medium">{{ category.name }}</div>
                  <div class="text-sm text-gray-500">
                    {{ category.description }}
                  </div>
                </div>
              </div>
            </button>
          </div>
        </UCard>
      </div>

      <!-- 配置内容区域 -->
      <div class="lg:col-span-3">
        <!-- 基础设置 -->
        <UCard v-if="selectedCategory === 'basic'">
          <template #header>
            <div class="flex items-center justify-between">
              <h3 class="text-lg font-semibold">基础设置</h3>
              <div class="space-x-2">
                <UButton variant="outline" @click="resetConfig('basic')"
                  >重置</UButton
                >
                <UButton @click="saveConfig('basic')">保存</UButton>
              </div>
            </div>
          </template>

          <UForm :state="basicForm" class="space-y-6">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
              <UFormField label="站点名称" name="siteName">
                <UInput
                  v-model="basicForm.siteName"
                  placeholder="请输入站点名称"
                />
              </UFormField>

              <UFormField label="管理员邮箱" name="adminEmail">
                <UInput
                  v-model="basicForm.adminEmail"
                  type="email"
                  placeholder="请输入管理员邮箱"
                />
              </UFormField>

              <UFormField label="站点URL" name="siteUrl">
                <UInput
                  v-model="basicForm.siteUrl"
                  placeholder="请输入站点URL"
                />
              </UFormField>

              <UFormField label="时区" name="timezone">
                <USelect
                  v-model="basicForm.timezone"
                  :items="timezoneOptions"
                />
              </UFormField>

              <UFormField label="默认语言" name="language">
                <USelect
                  v-model="basicForm.language"
                  :items="languageOptions"
                />
              </UFormField>

              <UFormField label="日期格式" name="dateFormat">
                <UInput
                  v-model="basicForm.dateFormat"
                  placeholder="YYYY-MM-DD"
                />
              </UFormField>
            </div>

            <UFormField label="站点描述" name="siteDescription">
              <UTextarea
                v-model="basicForm.siteDescription"
                placeholder="请输入站点描述"
              />
            </UFormField>
          </UForm>
        </UCard>

        <!-- 安全设置 -->
        <UCard v-if="selectedCategory === 'security'">
          <template #header>
            <div class="flex items-center justify-between">
              <h3 class="text-lg font-semibold">安全设置</h3>
              <div class="space-x-2">
                <UButton variant="outline" @click="resetConfig('security')"
                  >重置</UButton
                >
                <UButton @click="saveConfig('security')">保存</UButton>
              </div>
            </div>
          </template>

          <UForm :state="securityForm" class="space-y-6">
            <div class="space-y-4">
              <h4 class="font-medium text-gray-900">密码策略</h4>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <UFormField label="最小长度" name="passwordMinLength">
                  <UInput
                    v-model="securityForm.passwordMinLength"
                    type="number"
                    min="6"
                    max="32"
                  />
                </UFormField>

                <div class="space-y-3">
                  <UCheckbox
                    v-model="securityForm.passwordRequireUppercase"
                    label="要求大写字母"
                  />
                  <UCheckbox
                    v-model="securityForm.passwordRequireLowercase"
                    label="要求小写字母"
                  />
                  <UCheckbox
                    v-model="securityForm.passwordRequireNumbers"
                    label="要求数字"
                  />
                  <UCheckbox
                    v-model="securityForm.passwordRequireSymbols"
                    label="要求特殊字符"
                  />
                </div>
              </div>
            </div>

            <USeparator />

            <div class="space-y-4">
              <h4 class="font-medium text-gray-900">登录安全</h4>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <UFormField label="最大登录尝试次数" name="loginMaxAttempts">
                  <UInput
                    v-model="securityForm.loginMaxAttempts"
                    type="number"
                    min="3"
                    max="10"
                  />
                </UFormField>

                <UFormField
                  label="锁定时长（分钟）"
                  name="loginLockoutDuration"
                >
                  <UInput
                    v-model="securityForm.loginLockoutDuration"
                    type="number"
                    min="5"
                    max="1440"
                  />
                </UFormField>

                <UFormField label="会话超时（分钟）" name="sessionTimeout">
                  <UInput
                    v-model="securityForm.sessionTimeout"
                    type="number"
                    min="30"
                    max="1440"
                  />
                </UFormField>

                <div class="space-y-3">
                  <UCheckbox
                    v-model="securityForm.enableTwoFactor"
                    label="启用双因子认证"
                  />
                  <UCheckbox
                    v-model="securityForm.enableCaptcha"
                    label="启用验证码"
                  />
                </div>
              </div>
            </div>
          </UForm>
        </UCard>

        <!-- 通知设置 -->
        <UCard v-if="selectedCategory === 'notification'">
          <template #header>
            <div class="flex items-center justify-between">
              <h3 class="text-lg font-semibold">通知设置</h3>
              <div class="space-x-2">
                <UButton variant="outline" @click="resetConfig('notification')"
                  >重置</UButton
                >
                <UButton @click="saveConfig('notification')">保存</UButton>
              </div>
            </div>
          </template>

          <UForm :state="notificationForm" class="space-y-6">
            <!-- 邮件设置 -->
            <div class="space-y-4">
              <div class="flex items-center justify-between">
                <h4 class="font-medium text-gray-900">邮件设置</h4>
                <div class="flex items-center space-x-2">
                  <UCheckbox v-model="notificationForm.emailEnabled" />
                  <UButton
                    v-if="notificationForm.emailEnabled"
                    variant="outline"
                    size="sm"
                    @click="testConnection('邮件')"
                  >
                    测试连接
                  </UButton>
                </div>
              </div>

              <div
                v-if="notificationForm.emailEnabled"
                class="grid grid-cols-1 md:grid-cols-2 gap-6"
              >
                <UFormField label="SMTP服务器" name="emailHost">
                  <UInput
                    v-model="notificationForm.emailHost"
                    placeholder="smtp.example.com"
                  />
                </UFormField>

                <UFormField label="端口" name="emailPort">
                  <UInput
                    v-model="notificationForm.emailPort"
                    type="number"
                    placeholder="587"
                  />
                </UFormField>

                <UFormField label="用户名" name="emailUsername">
                  <UInput
                    v-model="notificationForm.emailUsername"
                    placeholder="请输入用户名"
                  />
                </UFormField>

                <UFormField label="密码" name="emailPassword">
                  <UInput
                    v-model="notificationForm.emailPassword"
                    type="password"
                    placeholder="请输入密码"
                  />
                </UFormField>

                <UFormField label="发件人名称" name="emailFromName">
                  <UInput
                    v-model="notificationForm.emailFromName"
                    placeholder="PowerX Admin"
                  />
                </UFormField>

                <UFormField label="发件人邮箱" name="emailFromAddress">
                  <UInput
                    v-model="notificationForm.emailFromAddress"
                    type="email"
                    placeholder="noreply@example.com"
                  />
                </UFormField>
              </div>
            </div>

            <USeparator />

            <!-- 短信设置 -->
            <div class="space-y-4">
              <div class="flex items-center justify-between">
                <h4 class="font-medium text-gray-900">短信设置</h4>
                <div class="flex items-center space-x-2">
                  <UCheckbox v-model="notificationForm.smsEnabled" />
                  <UButton
                    v-if="notificationForm.smsEnabled"
                    variant="outline"
                    size="sm"
                    @click="testConnection('短信')"
                  >
                    测试连接
                  </UButton>
                </div>
              </div>

              <div
                v-if="notificationForm.smsEnabled"
                class="grid grid-cols-1 md:grid-cols-2 gap-6"
              >
                <UFormField label="服务商" name="smsProvider">
                  <USelect
                    v-model="notificationForm.smsProvider"
                    :items="[
                      { label: '阿里云', value: 'aliyun' },
                      { label: '腾讯云', value: 'tencent' },
                      { label: '华为云', value: 'huawei' },
                    ]"
                  />
                </UFormField>

                <div></div>

                <UFormField label="Access Key" name="smsAccessKey">
                  <UInput
                    v-model="notificationForm.smsAccessKey"
                    placeholder="请输入Access Key"
                  />
                </UFormField>

                <UFormField label="Secret Key" name="smsSecretKey">
                  <UInput
                    v-model="notificationForm.smsSecretKey"
                    type="password"
                    placeholder="请输入Secret Key"
                  />
                </UFormField>
              </div>
            </div>

            <USeparator />

            <!-- 系统通知 -->
            <div class="space-y-4">
              <h4 class="font-medium text-gray-900">系统通知</h4>
              <div class="space-y-3">
                <UCheckbox
                  v-model="notificationForm.systemNotificationEnabled"
                  label="启用系统通知"
                />
                <UCheckbox
                  v-model="notificationForm.browserNotificationEnabled"
                  label="启用浏览器通知"
                />
              </div>
            </div>
          </UForm>
        </UCard>

        <!-- 存储设置 -->
        <UCard v-if="selectedCategory === 'storage'">
          <template #header>
            <div class="flex items-center justify-between">
              <h3 class="text-lg font-semibold">存储设置</h3>
              <div class="space-x-2">
                <UButton variant="outline" @click="resetConfig('storage')"
                  >重置</UButton
                >
                <UButton @click="saveConfig('storage')">保存</UButton>
              </div>
            </div>
          </template>

          <UForm :state="storageForm" class="space-y-6">
            <!-- 文件上传设置 -->
            <div class="space-y-4">
              <h4 class="font-medium text-gray-900">文件上传</h4>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <UFormField label="最大文件大小（MB）" name="uploadMaxSize">
                  <UInput
                    v-model="storageForm.uploadMaxSize"
                    type="number"
                    min="1"
                    max="1024"
                  />
                </UFormField>

                <UFormField label="存储驱动" name="storageDriver">
                  <USelect
                    v-model="storageForm.storageDriver"
                    :items="storageDriverOptions"
                  />
                </UFormField>
              </div>

              <UFormField label="允许的文件类型" name="uploadAllowedTypes">
                <UInput
                  :model-value="storageForm.uploadAllowedTypes.join(', ')"
                  @update:model-value="
                    storageForm.uploadAllowedTypes = $event
                      .split(',')
                      .map((s) => s.trim())
                  "
                  placeholder="jpg, png, pdf, doc"
                />
              </UFormField>

              <!-- 云存储配置 -->
              <div
                v-if="storageForm.storageDriver !== 'local'"
                class="grid grid-cols-1 md:grid-cols-2 gap-6"
              >
                <UFormField label="存储端点" name="storageEndpoint">
                  <UInput
                    v-model="storageForm.storageEndpoint"
                    placeholder="请输入存储端点"
                  />
                </UFormField>

                <UFormField label="存储桶名称" name="storageBucket">
                  <UInput
                    v-model="storageForm.storageBucket"
                    placeholder="请输入存储桶名称"
                  />
                </UFormField>

                <UFormField label="Access Key" name="storageAccessKey">
                  <UInput
                    v-model="storageForm.storageAccessKey"
                    placeholder="请输入Access Key"
                  />
                </UFormField>

                <UFormField label="Secret Key" name="storageSecretKey">
                  <UInput
                    v-model="storageForm.storageSecretKey"
                    type="password"
                    placeholder="请输入Secret Key"
                  />
                </UFormField>
              </div>
            </div>

            <USeparator />

            <!-- 缓存设置 -->
            <div class="space-y-4">
              <h4 class="font-medium text-gray-900">缓存设置</h4>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <UFormField label="缓存驱动" name="cacheDriver">
                  <USelect
                    v-model="storageForm.cacheDriver"
                    :items="cacheDriverOptions"
                  />
                </UFormField>

                <div></div>

                <UFormField
                  v-if="storageForm.cacheDriver === 'redis'"
                  label="Redis主机"
                  name="cacheHost"
                >
                  <UInput
                    v-model="storageForm.cacheHost"
                    placeholder="localhost"
                  />
                </UFormField>

                <UFormField
                  v-if="storageForm.cacheDriver === 'redis'"
                  label="Redis端口"
                  name="cachePort"
                >
                  <UInput
                    v-model="storageForm.cachePort"
                    type="number"
                    placeholder="6379"
                  />
                </UFormField>

                <UFormField
                  v-if="storageForm.cacheDriver === 'redis'"
                  label="Redis密码"
                  name="cachePassword"
                >
                  <UInput
                    v-model="storageForm.cachePassword"
                    type="password"
                    placeholder="请输入密码（可选）"
                  />
                </UFormField>
              </div>
            </div>
          </UForm>
        </UCard>

        <!-- 集成设置 -->
        <UCard v-if="selectedCategory === 'integration'">
          <template #header>
            <div class="flex items-center justify-between">
              <h3 class="text-lg font-semibold">集成设置</h3>
              <div class="space-x-2">
                <UButton variant="outline" @click="resetConfig('integration')"
                  >重置</UButton
                >
                <UButton @click="saveConfig('integration')">保存</UButton>
              </div>
            </div>
          </template>

          <UForm :state="integrationForm" class="space-y-6">
            <!-- 微信集成 -->
            <div class="space-y-4">
              <div class="flex items-center justify-between">
                <h4 class="font-medium text-gray-900">微信集成</h4>
                <UCheckbox v-model="integrationForm.wechatEnabled" />
              </div>

              <div
                v-if="integrationForm.wechatEnabled"
                class="grid grid-cols-1 md:grid-cols-2 gap-6"
              >
                <UFormField label="App ID" name="wechatAppId">
                  <UInput
                    v-model="integrationForm.wechatAppId"
                    placeholder="请输入微信App ID"
                  />
                </UFormField>

                <UFormField label="App Secret" name="wechatAppSecret">
                  <UInput
                    v-model="integrationForm.wechatAppSecret"
                    type="password"
                    placeholder="请输入微信App Secret"
                  />
                </UFormField>
              </div>
            </div>

            <USeparator />

            <!-- 支付宝集成 -->
            <div class="space-y-4">
              <div class="flex items-center justify-between">
                <h4 class="font-medium text-gray-900">支付宝集成</h4>
                <UCheckbox v-model="integrationForm.alipayEnabled" />
              </div>

              <div
                v-if="integrationForm.alipayEnabled"
                class="grid grid-cols-1 md:grid-cols-2 gap-6"
              >
                <UFormField label="App ID" name="alipayAppId">
                  <UInput
                    v-model="integrationForm.alipayAppId"
                    placeholder="请输入支付宝App ID"
                  />
                </UFormField>

                <div></div>

                <UFormField label="应用私钥" name="alipayPrivateKey">
                  <UTextarea
                    v-model="integrationForm.alipayPrivateKey"
                    placeholder="请输入应用私钥"
                  />
                </UFormField>

                <UFormField label="支付宝公钥" name="alipayPublicKey">
                  <UTextarea
                    v-model="integrationForm.alipayPublicKey"
                    placeholder="请输入支付宝公钥"
                  />
                </UFormField>
              </div>
            </div>

            <USeparator />

            <!-- 其他集成 -->
            <div class="space-y-4">
              <h4 class="font-medium text-gray-900">其他集成</h4>

              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div class="space-y-4">
                  <div class="flex items-center justify-between">
                    <span class="font-medium">数据分析</span>
                    <UCheckbox v-model="integrationForm.analyticsEnabled" />
                  </div>

                  <UFormField
                    v-if="integrationForm.analyticsEnabled"
                    label="跟踪ID"
                    name="analyticsTrackingId"
                  >
                    <UInput
                      v-model="integrationForm.analyticsTrackingId"
                      placeholder="GA-XXXXXXXXX-X"
                    />
                  </UFormField>
                </div>

                <div class="space-y-4">
                  <UFormField label="地图服务" name="mapProvider">
                    <USelect
                      v-model="integrationForm.mapProvider"
                      :items="[
                        { label: '百度地图', value: 'baidu' },
                        { label: '高德地图', value: 'amap' },
                        { label: '腾讯地图', value: 'tencent' },
                      ]"
                    />
                  </UFormField>

                  <UFormField label="API密钥" name="mapApiKey">
                    <UInput
                      v-model="integrationForm.mapApiKey"
                      placeholder="请输入地图API密钥"
                    />
                  </UFormField>
                </div>
              </div>
            </div>
          </UForm>
        </UCard>

        <!-- 高级设置 -->
        <UCard v-if="selectedCategory === 'advanced'">
          <template #header>
            <div class="flex items-center justify-between">
              <h3 class="text-lg font-semibold">高级设置</h3>
              <div class="space-x-2">
                <UButton variant="outline" @click="resetConfig('advanced')"
                  >重置</UButton
                >
                <UButton @click="saveConfig('advanced')">保存</UButton>
              </div>
            </div>
          </template>

          <UForm :state="advancedForm" class="space-y-6">
            <!-- 系统模式 -->
            <div class="space-y-4">
              <h4 class="font-medium text-gray-900">系统模式</h4>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div class="space-y-3">
                  <UCheckbox
                    v-model="advancedForm.debugMode"
                    label="调试模式"
                  />
                  <UCheckbox
                    v-model="advancedForm.maintenanceMode"
                    label="维护模式"
                  />
                  <UCheckbox
                    v-model="advancedForm.cacheEnabled"
                    label="启用缓存"
                  />
                  <UCheckbox
                    v-model="advancedForm.compressionEnabled"
                    label="启用压缩"
                  />
                </div>

                <UFormField
                  v-if="advancedForm.maintenanceMode"
                  label="维护提示信息"
                  name="maintenanceMessage"
                >
                  <UTextarea
                    v-model="advancedForm.maintenanceMessage"
                    placeholder="系统维护中，请稍后访问"
                  />
                </UFormField>
              </div>
            </div>

            <USeparator />

            <!-- 日志设置 -->
            <div class="space-y-4">
              <h4 class="font-medium text-gray-900">日志设置</h4>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <UFormField label="日志级别" name="logLevel">
                  <USelect
                    v-model="advancedForm.logLevel"
                    :items="logLevelOptions"
                  />
                </UFormField>

                <UFormField label="日志保留天数" name="logMaxFiles">
                  <UInput
                    v-model="advancedForm.logMaxFiles"
                    type="number"
                    min="1"
                    max="365"
                  />
                </UFormField>
              </div>
            </div>

            <USeparator />

            <!-- 性能监控 -->
            <div class="space-y-4">
              <h4 class="font-medium text-gray-900">性能监控</h4>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div class="space-y-3">
                  <UCheckbox
                    v-model="advancedForm.performanceMonitoring"
                    label="性能监控"
                  />
                  <UCheckbox
                    v-model="advancedForm.errorReporting"
                    label="错误报告"
                  />
                </div>
              </div>
            </div>

            <USeparator />

            <!-- 备份设置 -->
            <div class="space-y-4">
              <h4 class="font-medium text-gray-900">备份设置</h4>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div class="flex items-center space-x-2">
                  <UCheckbox v-model="advancedForm.backupEnabled" />
                  <span class="font-medium">启用自动备份</span>
                </div>

                <UFormField
                  v-if="advancedForm.backupEnabled"
                  label="备份频率"
                  name="backupFrequency"
                >
                  <USelect
                    v-model="advancedForm.backupFrequency"
                    :items="backupFrequencyOptions"
                  />
                </UFormField>
              </div>
            </div>
          </UForm>
        </UCard>

      </div>
    </div>
  </div>
</template>
