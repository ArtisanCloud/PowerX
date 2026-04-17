<script setup lang="ts">
definePageMeta({
  title: "备份恢复",
  icon: "i-heroicons-archive-box",
  order: 5,
});

const { t } = useI18n();

// 备份列表
const backups = ref([
  {
    id: 1,
    name: "backup_2024_01_15_full",
    type: "full",
    size: "2.5 GB",
    createdAt: "2024-01-15 02:00:00",
    status: "completed",
    description: "完整系统备份",
  },
  {
    id: 2,
    name: "backup_2024_01_14_incremental",
    type: "incremental",
    size: "156 MB",
    createdAt: "2024-01-14 02:00:00",
    status: "completed",
    description: "增量备份",
  },
  {
    id: 3,
    name: "backup_2024_01_13_database",
    type: "database",
    size: "89 MB",
    createdAt: "2024-01-13 02:00:00",
    status: "completed",
    description: "数据库备份",
  },
]);

// 备份配置
const backupConfig = reactive({
  autoBackup: true,
  backupTime: "02:00",
  backupType: "incremental",
  retentionDays: 30,
  includeFiles: true,
  includeDatabase: true,
  includeUploads: true,
  compressionLevel: "medium",
  encryptBackup: false,
  encryptionPassword: "",
});

// 创建备份状态
const isCreatingBackup = ref(false);
const backupProgress = ref(0);

// 恢复状态
const isRestoring = ref(false);
const restoreProgress = ref(0);

// 创建备份
const createBackup = async (type = "full") => {
  isCreatingBackup.value = true;
  backupProgress.value = 0;

  try {
    // 模拟备份进度
    const interval = setInterval(() => {
      backupProgress.value += Math.random() * 10;
      if (backupProgress.value >= 100) {
        backupProgress.value = 100;
        clearInterval(interval);
      }
    }, 500);

    // 模拟备份过程
    await new Promise((resolve) => setTimeout(resolve, 5000));

    // 添加新备份到列表
    const newBackup = {
      id: Date.now(),
      name: `backup_${new Date().toISOString().slice(0, 10)}_${type}`,
      type,
      size:
        type === "full" ? "2.8 GB" : type === "database" ? "95 MB" : "180 MB",
      createdAt: new Date().toLocaleString(),
      status: "completed",
      description:
        type === "full"
          ? "完整系统备份"
          : type === "database"
            ? "数据库备份"
            : "增量备份",
    };

    backups.value.unshift(newBackup);

    const toast = useToast();
    toast.add({
      title: "成功",
      description: "备份创建完成",
      color: "green",
    });
  } catch (error) {
    console.error("备份失败:", error);

    const toast = useToast();
    toast.add({
      title: "错误",
      description: "备份创建失败",
      color: "red",
    });
  } finally {
    isCreatingBackup.value = false;
    backupProgress.value = 0;
  }
};

// 恢复备份
const restoreBackup = async (backup) => {
  if (!confirm(`确定要恢复备份 "${backup.name}" 吗？此操作将覆盖当前数据！`)) {
    return;
  }

  isRestoring.value = true;
  restoreProgress.value = 0;

  try {
    // 模拟恢复进度
    const interval = setInterval(() => {
      restoreProgress.value += Math.random() * 8;
      if (restoreProgress.value >= 100) {
        restoreProgress.value = 100;
        clearInterval(interval);
      }
    }, 600);

    // 模拟恢复过程
    await new Promise((resolve) => setTimeout(resolve, 8000));

    const toast = useToast();
    toast.add({
      title: "成功",
      description: "备份恢复完成",
      color: "green",
    });
  } catch (error) {
    console.error("恢复失败:", error);

    const toast = useToast();
    toast.add({
      title: "错误",
      description: "备份恢复失败",
      color: "red",
    });
  } finally {
    isRestoring.value = false;
    restoreProgress.value = 0;
  }
};

// 删除备份
const deleteBackup = (id) => {
  if (confirm("确定要删除此备份吗？")) {
    backups.value = backups.value.filter((b) => b.id !== id);

    const toast = useToast();
    toast.add({
      title: "成功",
      description: "备份已删除",
      color: "green",
    });
  }
};

// 下载备份
const downloadBackup = (backup) => {
  // 模拟下载
  const toast = useToast();
  toast.add({
    title: "提示",
    description: `正在下载备份 ${backup.name}`,
    color: "blue",
  });
};

// 保存备份配置
const saveBackupConfig = async () => {
  try {
    // 这里应该调用 API 保存配置
    console.info("保存备份配置:", backupConfig);

    const toast = useToast();
    toast.add({
      title: "成功",
      description: "备份配置已保存",
      color: "green",
    });
  } catch (error) {
    console.error("保存配置失败:", error);

    const toast = useToast();
    toast.add({
      title: "错误",
      description: "保存配置失败",
      color: "red",
    });
  }
};

// 备份类型选项
const backupTypeOptions = [
  { label: "完整备份", value: "full" },
  { label: "增量备份", value: "incremental" },
  { label: "数据库备份", value: "database" },
];

// 压缩级别选项
const compressionOptions = [
  { label: "无压缩", value: "none" },
  { label: "低压缩", value: "low" },
  { label: "中等压缩", value: "medium" },
  { label: "高压缩", value: "high" },
];

// 获取备份类型标签
const getBackupTypeLabel = (type) => {
  const typeMap = {
    full: "完整",
    incremental: "增量",
    database: "数据库",
  };
  return typeMap[type] || type;
};

// 获取备份类型颜色
const getBackupTypeColor = (type) => {
  const colorMap = {
    full: "blue",
    incremental: "green",
    database: "orange",
  };
  return colorMap[type] || "gray";
};
</script>

<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900">备份恢复</h1>
      <p class="text-gray-600 mt-2">管理系统数据备份和恢复</p>
    </div>

    <div class="space-y-6">
      <!-- 快速操作 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">快速操作</h3>
        </template>

        <div class="flex flex-wrap gap-4">
          <UButton
            color="primary"
            icon="i-heroicons-archive-box-arrow-down"
            size="lg"
            :loading="isCreatingBackup"
            @click="createBackup('full')"
          >
            创建完整备份
          </UButton>

          <UButton
            color="success"
            icon="i-heroicons-archive-box-arrow-down"
            size="lg"
            variant="outline"
            :loading="isCreatingBackup"
            @click="createBackup('incremental')"
          >
            创建增量备份
          </UButton>

          <UButton
            color="orange"
            icon="i-heroicons-circle-stack"
            size="lg"
            variant="outline"
            :loading="isCreatingBackup"
            @click="createBackup('database')"
          >
            备份数据库
          </UButton>
        </div>

        <!-- 备份进度 -->
        <div v-if="isCreatingBackup" class="mt-4">
          <div class="flex items-center justify-between mb-2">
            <span class="text-sm font-medium">正在创建备份...</span>
            <span class="text-sm text-gray-500"
              >{{ Math.round(backupProgress) }}%</span
            >
          </div>
          <UProgress :value="backupProgress" color="primary" />
        </div>

        <!-- 恢复进度 -->
        <div v-if="isRestoring" class="mt-4">
          <div class="flex items-center justify-between mb-2">
            <span class="text-sm font-medium">正在恢复备份...</span>
            <span class="text-sm text-gray-500"
              >{{ Math.round(restoreProgress) }}%</span
            >
          </div>
          <UProgress :value="restoreProgress" color="success" />
        </div>
      </UCard>

      <!-- 备份列表 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">备份列表</h3>
        </template>

        <div class="space-y-4">
          <div
            v-for="backup in backups"
            :key="backup.id"
            class="flex items-center justify-between p-4 border rounded-lg hover:bg-gray-50"
          >
            <div class="flex-1">
              <div class="flex items-center gap-3 mb-2">
                <h4 class="font-medium">{{ backup.name }}</h4>
                <UBadge
                  :color="getBackupTypeColor(backup.type)"
                  variant="subtle"
                  size="sm"
                >
                  {{ getBackupTypeLabel(backup.type) }}
                </UBadge>
                <UBadge color="success" variant="subtle" size="sm">
                  {{ backup.status }}
                </UBadge>
              </div>
              <div class="text-sm text-gray-600">
                <p>{{ backup.description }}</p>
                <p>
                  大小: {{ backup.size }} | 创建时间: {{ backup.createdAt }}
                </p>
              </div>
            </div>

            <div class="flex items-center gap-2">
              <UButton
                color="primary"
                variant="outline"
                size="xs"
                icon="i-heroicons-arrow-down-tray"
                @click="downloadBackup(backup)"
              >
                下载
              </UButton>

              <UButton
                color="success"
                variant="outline"
                size="xs"
                icon="i-heroicons-arrow-path"
                :disabled="isRestoring"
                @click="restoreBackup(backup)"
              >
                恢复
              </UButton>

              <UButton
                color="error"
                variant="outline"
                size="xs"
                icon="i-heroicons-trash"
                @click="deleteBackup(backup.id)"
              >
                删除
              </UButton>
            </div>
          </div>

          <div
            v-if="backups.length === 0"
            class="text-center py-8 text-gray-500"
          >
            <UIcon
              name="i-heroicons-archive-box-x-mark"
              class="w-12 h-12 mx-auto mb-4"
            />
            <p>暂无备份文件</p>
          </div>
        </div>
      </UCard>

      <!-- 自动备份配置 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">自动备份配置</h3>
        </template>

        <div class="space-y-6">
          <!-- 基本设置 -->
          <div>
            <h4 class="font-medium mb-4">基本设置</h4>
            <div class="space-y-4">
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium">启用自动备份</label>
                  <p class="text-sm text-gray-600">定时自动创建系统备份</p>
                </div>
                <USwitch v-model="backupConfig.autoBackup" />
              </div>

              <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                <UFormField label="备份时间">
                  <UInput v-model="backupConfig.backupTime" type="time" />
                </UFormField>

                <UFormField label="备份类型">
                  <USelect
                    v-model="backupConfig.backupType"
                    :items="backupTypeOptions"
                  />
                </UFormField>

                <UFormField label="保留天数">
                  <UInput
                    v-model="backupConfig.retentionDays"
                    type="number"
                    min="1"
                    max="365"
                  />
                </UFormField>
              </div>
            </div>
          </div>

          <!-- 备份内容 -->
          <div>
            <h4 class="font-medium mb-4">备份内容</h4>
            <div class="space-y-3">
              <UCheckbox
                v-model="backupConfig.includeDatabase"
                label="包含数据库"
              />
              <UCheckbox
                v-model="backupConfig.includeFiles"
                label="包含系统文件"
              />
              <UCheckbox
                v-model="backupConfig.includeUploads"
                label="包含上传文件"
              />
            </div>
          </div>

          <!-- 高级设置 -->
          <div>
            <h4 class="font-medium mb-4">高级设置</h4>
            <div class="space-y-4">
              <UFormField label="压缩级别">
                <USelect
                  v-model="backupConfig.compressionLevel"
                  :items="compressionOptions"
                />
              </UFormField>

              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium">加密备份</label>
                  <p class="text-sm text-gray-600">使用密码加密备份文件</p>
                </div>
                <USwitch v-model="backupConfig.encryptBackup" />
              </div>

              <UFormField
                v-if="backupConfig.encryptBackup"
                label="加密密码"
                required
              >
                <UInput
                  v-model="backupConfig.encryptionPassword"
                  type="password"
                  placeholder="输入加密密码"
                />
              </UFormField>
            </div>
          </div>

          <!-- 保存按钮 -->
          <div class="flex justify-end pt-4 border-t">
            <UButton color="primary" @click="saveBackupConfig">
              保存配置
            </UButton>
          </div>
        </div>
      </UCard>
    </div>
  </div>
</template>
