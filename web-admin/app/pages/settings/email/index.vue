<script setup lang="ts">
definePageMeta({
  title: "邮件设置",
  icon: "i-heroicons-envelope",
  order: 4,
});

const { t } = useI18n();

// 邮件配置
const emailConfig = reactive({
  // SMTP 配置
  smtp: {
    host: "smtp.gmail.com",
    port: 587,
    secure: false,
    username: "",
    password: "",
    fromName: "PowerX Admin",
    fromEmail: "noreply@powerx.com",
  },
  // 邮件模板配置
  templates: {
    welcome: {
      subject: "欢迎加入 PowerX",
      enabled: true,
    },
    resetPassword: {
      subject: "重置密码",
      enabled: true,
    },
    notification: {
      subject: "系统通知",
      enabled: true,
    },
  },
  // 发送设置
  settings: {
    enableQueue: true,
    maxRetries: 3,
    retryDelay: 300,
    dailyLimit: 1000,
  },
});

// 测试邮件配置
const testEmail = ref("");
const isTestingSMTP = ref(false);

// 保存邮件配置
const saveEmailConfig = async () => {
  try {
    // 这里应该调用 API 保存配置
    console.log("保存邮件配置:", emailConfig);

    const toast = useToast();
    toast.add({
      title: "成功",
      description: "邮件配置已保存",
      color: "success",
    });
  } catch (error) {
    console.error("保存邮件配置失败:", error);

    const toast = useToast();
    toast.add({
      title: "错误",
      description: "保存配置失败，请重试",
      color: "error",
    });
  }
};

// 测试 SMTP 连接
const testSMTPConnection = async () => {
  if (!testEmail.value) {
    const toast = useToast();
    toast.add({
      title: "错误",
      description: "请输入测试邮箱地址",
      color: "error",
    });
    return;
  }

  isTestingSMTP.value = true;

  try {
    // 这里应该调用 API 测试 SMTP 连接
    await new Promise((resolve) => setTimeout(resolve, 2000)); // 模拟测试

    const toast = useToast();
    toast.add({
      title: "成功",
      description: `测试邮件已发送到 ${testEmail.value}`,
      color: "success",
    });
  } catch (error) {
    console.error("SMTP 测试失败:", error);

    const toast = useToast();
    toast.add({
      title: "错误",
      description: "SMTP 连接测试失败",
      color: "error",
    });
  } finally {
    isTestingSMTP.value = false;
  }
};
</script>

<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900">邮件设置</h1>
      <p class="text-gray-600 mt-2">配置邮件服务器和邮件模板</p>
    </div>

    <div class="space-y-6">
      <!-- SMTP 配置 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">SMTP 服务器配置</h3>
        </template>

        <div class="space-y-4">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <UFormField label="SMTP 主机" required>
              <UInput
                v-model="emailConfig.smtp.host"
                placeholder="smtp.gmail.com"
              />
            </UFormField>

            <UFormField label="端口" required>
              <UInput
                v-model="emailConfig.smtp.port"
                type="number"
                placeholder="587"
              />
            </UFormField>

            <UFormField label="用户名" required>
              <UInput
                v-model="emailConfig.smtp.username"
                placeholder="your-email@gmail.com"
              />
            </UFormField>

            <UFormField label="密码" required>
              <UInput
                v-model="emailConfig.smtp.password"
                type="password"
                placeholder="应用专用密码"
              />
            </UFormField>

            <UFormField label="发件人姓名">
              <UInput
                v-model="emailConfig.smtp.fromName"
                placeholder="PowerX Admin"
              />
            </UFormField>

            <UFormField label="发件人邮箱">
              <UInput
                v-model="emailConfig.smtp.fromEmail"
                placeholder="noreply@powerx.com"
              />
            </UFormField>
          </div>

          <div class="flex items-center">
            <UCheckbox
              v-model="emailConfig.smtp.secure"
              label="使用 SSL/TLS 加密"
            />
          </div>

          <!-- SMTP 测试 -->
          <div class="border-t pt-4">
            <h4 class="font-medium mb-3">测试 SMTP 连接</h4>
            <div class="flex gap-3">
              <UInput
                v-model="testEmail"
                placeholder="输入测试邮箱地址"
                class="flex-1"
              />
              <UButton
                color="primary"
                :loading="isTestingSMTP"
                @click="testSMTPConnection"
              >
                发送测试邮件
              </UButton>
            </div>
          </div>
        </div>
      </UCard>

      <!-- 邮件模板配置 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">邮件模板配置</h3>
        </template>

        <div class="space-y-4">
          <div
            v-for="(template, key) in emailConfig.templates"
            :key="key"
            class="flex items-center justify-between p-4 border rounded-lg"
          >
            <div class="flex-1">
              <h4 class="font-medium">{{ template.subject }}</h4>
              <p class="text-sm text-gray-600">
                {{
                  key === "welcome"
                    ? "新用户注册欢迎邮件"
                    : key === "resetPassword"
                      ? "密码重置邮件"
                      : "系统通知邮件"
                }}
              </p>
            </div>
            <div class="flex items-center gap-3">
              <USwitch v-model="template.enabled" />
              <UButton color="neutral" variant="outline" size="xs">
                编辑模板
              </UButton>
            </div>
          </div>
        </div>
      </UCard>

      <!-- 发送设置 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">发送设置</h3>
        </template>

        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <label class="font-medium">启用邮件队列</label>
              <p class="text-sm text-gray-600">使用队列异步发送邮件</p>
            </div>
            <USwitch v-model="emailConfig.settings.enableQueue" />
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <UFormField label="最大重试次数">
              <UInput
                v-model="emailConfig.settings.maxRetries"
                type="number"
                min="1"
                max="10"
              />
            </UFormField>

            <UFormField label="重试延迟（秒）">
              <UInput
                v-model="emailConfig.settings.retryDelay"
                type="number"
                min="60"
                max="3600"
              />
            </UFormField>

            <UFormField label="每日发送限制">
              <UInput
                v-model="emailConfig.settings.dailyLimit"
                type="number"
                min="100"
                max="10000"
              />
            </UFormField>
          </div>
        </div>
      </UCard>

      <!-- 保存按钮 -->
      <div class="flex justify-end">
        <UButton color="primary" size="lg" @click="saveEmailConfig">
          保存邮件配置
        </UButton>
      </div>
    </div>
  </div>
</template>
