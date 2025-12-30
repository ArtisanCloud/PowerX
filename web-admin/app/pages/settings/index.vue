<template>
  <div class="p-6">
    <div class="mb-6 flex justify-between items-start">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
          系统设置
        </h1>
        <p class="text-gray-600 dark:text-gray-400">管理系统配置和偏好设置</p>
      </div>
      <ShowGuideButton />
    </div>

    <!-- 设置导航 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 mb-8">
      <UCard
        v-for="setting in settingCategories"
        :key="setting.key"
        class="cursor-pointer hover:shadow-md transition-shadow"
        @click="navigateTo(localePath(setting.path))"
      >
        <div class="flex items-center gap-4">
          <div class="p-3 rounded-lg" :class="setting.iconBg">
            <UIcon
              :name="setting.icon"
              class="w-6 h-6 setting.iconColor inline-block"
            />
          </div>
          <div>
            <h3 class="font-semibold text-gray-900 dark:text-white">
              {{ setting.title }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ setting.description }}
            </p>
          </div>
        </div>
      </UCard>
    </div>

    <!-- 快速设置 -->
    <UCard>
      <template #header>
        <h3 class="text-lg font-semibold">快速设置</h3>
      </template>

      <div class="space-y-6">
        <!-- 基本设置 -->
        <div>
          <h4 class="font-medium mb-4">基本设置</h4>
          <div class="space-y-4">
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium">网站名称</label>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  设置网站的显示名称
                </p>
              </div>
              <UInput
                v-model="settings.siteName"
                placeholder="网站名称"
                class="w-64"
              />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium">网站描述</label>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  网站的简短描述
                </p>
              </div>
              <UInput
                v-model="settings.siteDescription"
                placeholder="网站描述"
                class="w-64"
              />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium">默认语言</label>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  系统默认显示语言
                </p>
              </div>
              <USelect
                v-model="settings.defaultLanguage"
                :items="languageOptions"
                class="w-64"
              />
            </div>
          </div>
        </div>

        <!-- 功能开关 -->
        <div>
          <h4 class="font-medium mb-4">功能开关</h4>
          <div class="space-y-4">
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium">用户注册</label>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  允许新用户注册账号
                </p>
              </div>
              <USwitch v-model="settings.allowRegistration" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium">评论功能</label>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  启用文章评论功能
                </p>
              </div>
              <USwitch v-model="settings.enableComments" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium">邮件通知</label>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  发送系统邮件通知
                </p>
              </div>
              <USwitch v-model="settings.emailNotifications" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium">维护模式</label>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  启用网站维护模式
                </p>
              </div>
              <USwitch v-model="settings.maintenanceMode" />
            </div>
          </div>
        </div>

        <!-- 安全设置 -->
        <div>
          <h4 class="font-medium mb-4">安全设置</h4>
          <div class="space-y-4">
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium">强制 HTTPS</label>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  强制使用 HTTPS 连接
                </p>
              </div>
              <USwitch v-model="settings.forceHttps" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium">两步验证</label>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  启用两步验证功能
                </p>
              </div>
              <USwitch v-model="settings.twoFactorAuth" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium">登录尝试限制</label>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  限制登录失败次数
                </p>
              </div>
              <UInput
                v-model="settings.maxLoginAttempts"
                type="number"
                placeholder="5"
                class="w-20"
              />
            </div>
          </div>
        </div>

        <!-- 保存按钮 -->
        <div
          class="flex justify-end pt-4 border-t border-gray-200 dark:border-gray-700"
        >
          <div class="flex gap-2">
            <UButton variant="outline" @click="resetSettings">重置</UButton>
            <UButton color="primary" @click="saveSettings">保存设置</UButton>
          </div>
        </div>
      </div>
    </UCard>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive } from "vue";
import { storeToRefs } from "pinia";
import ShowGuideButton from "~/components/ui/ShowGuideButton.vue";
import { useUserStore } from "~/stores/user";

// 页面元数据
definePageMeta({
  title: "系统设置",
  layout: "default",
});

// 国际化
const localePath = useLocalePath();
const userStore = useUserStore();
const { isRoot } = storeToRefs(userStore);

// 设置分类
const baseSettingCategories = [
  {
    key: "users",
    title: "用户管理",
    description: "管理用户账号和权限",
    icon: "i-heroicons-users",
    iconBg: "bg-blue-50 dark:bg-blue-900/20",
    iconColor: "text-blue-600",
    path: "/settings/users",
  },
  {
    key: "roles",
    title: "角色权限",
    description: "配置用户角色和权限",
    icon: "i-heroicons-shield-check",
    iconBg: "bg-green-50 dark:bg-green-900/20",
    iconColor: "text-green-600",
    path: "/settings/roles",
  },
  {
    key: "config",
    title: "系统配置",
    description: "基础系统配置选项",
    icon: "i-heroicons-cog-6-tooth",
    iconBg: "bg-purple-50 dark:bg-purple-900/20",
    iconColor: "text-purple-600",
    path: "/settings/config",
  },
  {
    key: "email",
    title: "邮件设置",
    description: "配置邮件服务器和模板",
    icon: "i-heroicons-envelope",
    iconBg: "bg-orange-50 dark:bg-orange-900/20",
    iconColor: "text-orange-600",
    path: "/settings/email",
  },
  {
    key: "backup",
    title: "备份恢复",
    description: "数据备份和恢复管理",
    icon: "i-heroicons-archive-box",
    iconBg: "bg-red-50 dark:bg-red-900/20",
    iconColor: "text-red-600",
    path: "/settings/backup",
  },
  {
    key: "logs",
    title: "系统日志",
    description: "查看系统操作日志",
    icon: "i-heroicons-document-text",
    iconBg: "bg-gray-50 dark:bg-gray-900/20",
    iconColor: "text-gray-600",
    path: "/settings/logs",
  },
];

const settingCategories = computed(() => {
  const items = [...baseSettingCategories];
  if (isRoot.value) {
    items.push({
      key: "open-capabilities",
      title: "开放能力",
      description: "查看 CoreX 底座能力与调试入口",
      icon: "i-heroicons-bolt",
      iconBg: "bg-yellow-50 dark:bg-yellow-900/20",
      iconColor: "text-yellow-600",
      path: "/settings/open-capabilities",
    });
  }
  return items;
});

// 设置数据
const settings = reactive({
  siteName: "PowerX Admin",
  siteDescription: "强大的管理系统",
  defaultLanguage: "zh",
  allowRegistration: true,
  enableComments: true,
  emailNotifications: true,
  maintenanceMode: false,
  forceHttps: true,
  twoFactorAuth: false,
  maxLoginAttempts: 5,
});

// 语言选项
const languageOptions = [
  { label: "中文", value: "zh" },
  { label: "English", value: "en" },
  { label: "日本語", value: "ja" },
  { label: "한국어", value: "ko" },
];

// 保存设置
async function saveSettings() {
  try {
    // 这里应该调用 API 保存设置
    console.log("保存设置:", settings);

    const toast = useToast();
    toast.add({
      title: "成功",
      description: "设置已保存",
      color: "success",
    });
  } catch (error) {
    console.error("保存设置失败:", error);

    const toast = useToast();
    toast.add({
      title: "错误",
      description: "保存设置失败，请重试",
      color: "error",
    });
  }
}

// 重置设置
function resetSettings() {
  // 重置为默认值
  Object.assign(settings, {
    siteName: "PowerX Admin",
    siteDescription: "强大的管理系统",
    defaultLanguage: "zh",
    allowRegistration: true,
    enableComments: true,
    emailNotifications: true,
    maintenanceMode: false,
    forceHttps: true,
    twoFactorAuth: false,
    maxLoginAttempts: 5,
  });

  const toast = useToast();
  toast.add({
    title: "提示",
    description: "设置已重置为默认值",
    color: "primary",
  });
}
</script>
