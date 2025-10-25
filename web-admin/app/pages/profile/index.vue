<template>
  <div class="min-h-screen bg-gray-50 dark:bg-gray-900">
    <div class="max-w-4xl mx-auto px-4 py-8">
      <!-- 页面标题 -->
      <div class="mb-8">
        <h1 class="text-3xl font-bold text-gray-900 dark:text-white">
          {{ t("profile.title") }}
        </h1>
        <p class="mt-2 text-gray-600 dark:text-gray-400">
          {{ t("profile.subtitle") }}
        </p>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-3 gap-8">
        <!-- 左侧：个人信息卡片 -->
        <div class="lg:col-span-1">
          <UCard>
            <template #header>
              <h2 class="text-lg font-semibold">
                {{ t("profile.personalInfo") }}
              </h2>
            </template>

            <div class="text-center">
              <!-- 头像 -->
              <div class="relative inline-block mb-4">
                <div
                  class="w-24 h-24 bg-gray-300 dark:bg-gray-600 rounded-full flex items-center justify-center mx-auto"
                >
                  <UIcon
                    name="i-heroicons-user"
                    class="w-12 h-12 text-gray-600 dark:text-gray-300"
                  />
                </div>
                <UButton
                  variant="outline"
                  size="xs"
                  class="absolute bottom-0 right-0"
                  icon="i-heroicons-camera"
                  @click="handleAvatarUpload"
                />
              </div>

              <!-- 基本信息 -->
              <h3
                class="text-xl font-semibold text-gray-900 dark:text-white mb-1"
              >
                {{ userProfile.name }}
              </h3>
              <p class="text-gray-600 dark:text-gray-400 mb-2">
                {{ userProfile.email }}
              </p>
              <UBadge
                :label="userProfile.role"
                :color="getRoleColor(userProfile.role)"
                variant="soft"
                class="mb-4"
              />

              <!-- 统计信息 -->
              <div
                class="grid grid-cols-2 gap-4 pt-4 border-t border-gray-200 dark:border-gray-700"
              >
                <div class="text-center">
                  <div class="text-2xl font-bold text-gray-900 dark:text-white">
                    {{ userStats.loginCount }}
                  </div>
                  <div class="text-sm text-gray-600 dark:text-gray-400">
                    {{ t("profile.loginCount") }}
                  </div>
                </div>
                <div class="text-center">
                  <div class="text-2xl font-bold text-gray-900 dark:text-white">
                    {{ userStats.daysActive }}
                  </div>
                  <div class="text-sm text-gray-600 dark:text-gray-400">
                    {{ t("profile.activeDays") }}
                  </div>
                </div>
              </div>
            </div>
          </UCard>

          <!-- 快速操作 -->
          <UCard class="mt-6">
            <template #header>
              <h2 class="text-lg font-semibold">
                {{ t("profile.quickActions") }}
              </h2>
            </template>

            <div class="space-y-3">
              <UButton
                variant="outline"
                block
                icon="i-heroicons-key"
                @click="showPasswordModal = true"
              >
                {{ t("profile.changePassword") }}
              </UButton>
              <UButton
                variant="outline"
                block
                icon="i-heroicons-bell"
                :to="'/notifications'"
              >
                {{ t("profile.notifications") }}
              </UButton>
              <UButton
                variant="outline"
                block
                icon="i-heroicons-cog-6-tooth"
                :to="'/settings'"
              >
                {{ t("profile.settings") }}
              </UButton>
            </div>
          </UCard>
        </div>

        <!-- 右侧：详细信息和设置 -->
        <div class="lg:col-span-2 space-y-6">
          <!-- 个人信息编辑 -->
          <UCard>
            <template #header>
              <div class="flex items-center justify-between">
                <h2 class="text-lg font-semibold">
                  {{ t("profile.editProfile") }}
                </h2>
                <UButton
                  v-if="!isEditing"
                  variant="outline"
                  size="sm"
                  icon="i-heroicons-pencil"
                  @click="startEditing"
                >
                  {{ t("profile.edit") }}
                </UButton>
              </div>
            </template>

            <UForm
              :schema="profileSchema"
              :state="editForm"
              @submit="handleSaveProfile"
            >
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <UFormField :label="t('profile.name')" name="name">
                  <UInput
                    v-model="editForm.name"
                    :disabled="!isEditing"
                    icon="i-heroicons-user"
                  />
                </UFormField>

                <UFormField :label="t('profile.email')" name="email">
                  <UInput
                    v-model="editForm.email"
                    :disabled="!isEditing"
                    icon="i-heroicons-envelope"
                    type="email"
                  />
                </UFormField>

                <UFormField :label="t('profile.phone')" name="phone">
                  <UInput
                    v-model="editForm.phone"
                    :disabled="!isEditing"
                    icon="i-heroicons-phone"
                  />
                </UFormField>

                <UFormField :label="t('profile.department')" name="department">
                  <UInput
                    v-model="editForm.department"
                    :disabled="!isEditing"
                    icon="i-heroicons-building-office"
                  />
                </UFormField>

                <UFormField
                  :label="t('profile.position')"
                  name="position"
                  class="md:col-span-2"
                >
                  <UInput
                    v-model="editForm.position"
                    :disabled="!isEditing"
                    icon="i-heroicons-briefcase"
                  />
                </UFormField>

                <UFormField
                  :label="t('profile.bio')"
                  name="bio"
                  class="md:col-span-2"
                >
                  <UTextarea
                    v-model="editForm.bio"
                    :disabled="!isEditing"
                    :rows="3"
                    :placeholder="t('profile.bioPlaceholder')"
                  />
                </UFormField>
              </div>

              <div
                v-if="isEditing"
                class="flex justify-end space-x-3 mt-6 pt-6 border-t border-gray-200 dark:border-gray-700"
              >
                <UButton variant="outline" @click="cancelEditing">
                  {{ t("common.cancel") }}
                </UButton>
                <UButton type="submit" :loading="isSaving">
                  {{ t("common.save") }}
                </UButton>
              </div>
            </UForm>
          </UCard>

          <!-- 安全设置 -->
          <UCard>
            <template #header>
              <h2 class="text-lg font-semibold">{{ t("profile.security") }}</h2>
            </template>

            <div class="space-y-4">
              <div
                class="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-800 rounded-lg"
              >
                <div>
                  <h3 class="font-medium text-gray-900 dark:text-white">
                    {{ t("profile.twoFactorAuth") }}
                  </h3>
                  <p class="text-sm text-gray-600 dark:text-gray-400">
                    {{ t("profile.twoFactorAuthDesc") }}
                  </p>
                </div>
                <USwitch
                  v-model="securitySettings.twoFactorEnabled"
                  @change="handleTwoFactorToggle"
                />
              </div>

              <div
                class="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-800 rounded-lg"
              >
                <div>
                  <h3 class="font-medium text-gray-900 dark:text-white">
                    {{ t("profile.emailNotifications") }}
                  </h3>
                  <p class="text-sm text-gray-600 dark:text-gray-400">
                    {{ t("profile.emailNotificationsDesc") }}
                  </p>
                </div>
                <USwitch
                  v-model="securitySettings.emailNotifications"
                  @change="handleEmailNotificationToggle"
                />
              </div>

              <div class="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
                <div class="flex items-center justify-between mb-2">
                  <h3 class="font-medium text-gray-900 dark:text-white">
                    {{ t("profile.lastLogin") }}
                  </h3>
                </div>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  {{ formatDateTime(userProfile.lastLoginAt) }}
                </p>
                <p class="text-xs text-gray-500 dark:text-gray-500 mt-1">
                  IP: {{ userProfile.lastLoginIp }}
                </p>
              </div>
            </div>
          </UCard>

          <!-- 活动记录 -->
          <UCard>
            <template #header>
              <h2 class="text-lg font-semibold">
                {{ t("profile.recentActivity") }}
              </h2>
            </template>

            <div class="space-y-4">
              <div
                v-for="activity in recentActivities"
                :key="activity.id"
                class="flex items-start space-x-3 p-3 hover:bg-gray-50 dark:hover:bg-gray-800 rounded-lg"
              >
                <div class="flex-shrink-0">
                  <div
                    class="w-8 h-8 bg-blue-100 dark:bg-blue-900 rounded-full flex items-center justify-center"
                  >
                    <UIcon
                      :name="getActivityIcon(activity.type)"
                      class="w-4 h-4 text-blue-600 dark:text-blue-400"
                    />
                  </div>
                </div>
                <div class="flex-1 min-w-0">
                  <p class="text-sm text-gray-900 dark:text-white">
                    {{ activity.description }}
                  </p>
                  <p class="text-xs text-gray-500 dark:text-gray-400">
                    {{ formatDateTime(activity.createdAt) }}
                  </p>
                </div>
              </div>
            </div>
          </UCard>
        </div>
      </div>
    </div>

    <!-- 修改密码模态框 -->
    <UModal
      v-model="showPasswordModal"
      title="profile-title"
      description="profile-description"
    >
      <template #content>
        <UCard>
          <template #header>
            <h3 class="text-lg font-semibold">
              {{ t("profile.changePassword") }}
            </h3>
          </template>

          <UForm
            :schema="passwordSchema"
            :state="passwordForm"
            @submit="handleChangePassword"
          >
            <div class="space-y-4">
              <UFormField
                :label="t('profile.currentPassword')"
                name="currentPassword"
              >
                <UInput
                  v-model="passwordForm.currentPassword"
                  type="password"
                  icon="i-heroicons-lock-closed"
                />
              </UFormField>

              <UFormField :label="t('profile.newPassword')" name="newPassword">
                <UInput
                  v-model="passwordForm.newPassword"
                  type="password"
                  icon="i-heroicons-key"
                />
              </UFormField>

              <UFormField
                :label="t('profile.confirmPassword')"
                name="confirmPassword"
              >
                <UInput
                  v-model="passwordForm.confirmPassword"
                  type="password"
                  icon="i-heroicons-key"
                />
              </UFormField>
            </div>

            <div class="flex justify-end space-x-3 mt-6">
              <UButton variant="outline" @click="showPasswordModal = false">
                {{ t("common.cancel") }}
              </UButton>
              <UButton type="submit" :loading="isChangingPassword">
                {{ t("profile.changePassword") }}
              </UButton>
            </div>
          </UForm>
        </UCard>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { z } from "zod";

// 页面元数据
definePageMeta({
  title: "个人资料",
  description: "管理个人资料和账户设置",
});

const { t } = useI18n();

// 响应式数据
const isEditing = ref(false);
const isSaving = ref(false);
const showPasswordModal = ref(false);
const isChangingPassword = ref(false);

// 用户资料数据
const userProfile = ref({
  id: "1",
  name: "管理员",
  email: "admin@powerx.com",
  phone: "+86 138 0013 8000",
  department: "技术部",
  position: "系统管理员",
  bio: "负责系统运维和技术支持工作",
  role: "管理员",
  avatar: null,
  lastLoginAt: new Date("2024-03-10T10:30:00"),
  lastLoginIp: "192.168.1.100",
  createdAt: new Date("2024-01-01"),
  updatedAt: new Date("2024-03-10"),
});

// 用户统计数据
const userStats = ref({
  loginCount: 156,
  daysActive: 45,
});

// 编辑表单
const editForm = ref({
  name: userProfile.value.name,
  email: userProfile.value.email,
  phone: userProfile.value.phone,
  department: userProfile.value.department,
  position: userProfile.value.position,
  bio: userProfile.value.bio,
});

// 安全设置
const securitySettings = ref({
  twoFactorEnabled: false,
  emailNotifications: true,
});

// 密码修改表单
const passwordForm = ref({
  currentPassword: "",
  newPassword: "",
  confirmPassword: "",
});

// 最近活动
const recentActivities = ref([
  {
    id: "1",
    type: "login",
    description: "登录系统",
    createdAt: new Date("2024-03-10T10:30:00"),
  },
  {
    id: "2",
    type: "update",
    description: "更新了个人资料",
    createdAt: new Date("2024-03-09T15:20:00"),
  },
  {
    id: "3",
    type: "search",
    description: "执行了搜索操作",
    createdAt: new Date("2024-03-09T14:15:00"),
  },
  {
    id: "4",
    type: "setting",
    description: "修改了系统设置",
    createdAt: new Date("2024-03-08T09:45:00"),
  },
]);

// 表单验证模式
const profileSchema = z.object({
  name: z.string().min(1, "姓名不能为空"),
  email: z.string().email("请输入有效的邮箱地址"),
  phone: z.string().optional(),
  department: z.string().optional(),
  position: z.string().optional(),
  bio: z.string().optional(),
});

const passwordSchema = z
  .object({
    currentPassword: z.string().min(1, "请输入当前密码"),
    newPassword: z.string().min(6, "新密码至少6位"),
    confirmPassword: z.string().min(1, "请确认新密码"),
  })
  .refine((data) => data.newPassword === data.confirmPassword, {
    message: "两次输入的密码不一致",
    path: ["confirmPassword"],
  });

// 方法
const startEditing = () => {
  isEditing.value = true;
};

const cancelEditing = () => {
  isEditing.value = false;
  // 重置表单
  editForm.value = {
    name: userProfile.value.name,
    email: userProfile.value.email,
    phone: userProfile.value.phone,
    department: userProfile.value.department,
    position: userProfile.value.position,
    bio: userProfile.value.bio,
  };
};

const handleSaveProfile = async () => {
  isSaving.value = true;
  try {
    // 模拟API调用
    await new Promise((resolve) => setTimeout(resolve, 1000));

    // 更新用户资料
    Object.assign(userProfile.value, editForm.value);
    userProfile.value.updatedAt = new Date();

    isEditing.value = false;

    // 显示成功消息
    const toast = useToast();
    toast.add({
      title: "保存成功",
      description: "个人资料已更新",
      color: "green",
    });
  } catch (error) {
    console.error("保存失败:", error);
    const toast = useToast();
    toast.add({
      title: "保存失败",
      description: "请稍后重试",
      color: "red",
    });
  } finally {
    isSaving.value = false;
  }
};

const handleChangePassword = async () => {
  isChangingPassword.value = true;
  try {
    // 模拟API调用
    await new Promise((resolve) => setTimeout(resolve, 1000));

    showPasswordModal.value = false;
    passwordForm.value = {
      currentPassword: "",
      newPassword: "",
      confirmPassword: "",
    };

    const toast = useToast();
    toast.add({
      title: "密码修改成功",
      description: "请使用新密码登录",
      color: "green",
    });
  } catch (error) {
    console.error("密码修改失败:", error);
    const toast = useToast();
    toast.add({
      title: "密码修改失败",
      description: "请检查当前密码是否正确",
      color: "red",
    });
  } finally {
    isChangingPassword.value = false;
  }
};

const handleAvatarUpload = () => {
  // 头像上传逻辑
  console.log("上传头像");
};

const handleTwoFactorToggle = (enabled: boolean) => {
  console.log("双因子认证:", enabled);
  // 处理双因子认证设置
};

const handleEmailNotificationToggle = (enabled: boolean) => {
  console.log("邮件通知:", enabled);
  // 处理邮件通知设置
};

// 工具函数
const getRoleColor = (role: string) => {
  switch (role) {
    case "管理员":
      return "red";
    case "编辑者":
      return "blue";
    case "查看者":
      return "green";
    default:
      return "gray";
  }
};

const getActivityIcon = (type: string) => {
  switch (type) {
    case "login":
      return "i-heroicons-arrow-right-on-rectangle";
    case "update":
      return "i-heroicons-pencil";
    case "search":
      return "i-heroicons-magnifying-glass";
    case "setting":
      return "i-heroicons-cog-6-tooth";
    default:
      return "i-heroicons-information-circle";
  }
};

const formatDateTime = (date: Date) => {
  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
};
</script>
