<template>
  <div class="offline-packages">
    <div class="page-header">
      <h1>离线包入库</h1>
      <p class="text-gray-600">提交离线包进行 Marketplace 审核</p>
    </div>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">提交离线包</h3>
        </div>
      </template>

      <UForm :state="formState" @submit="handleSubmit">
        <div class="space-y-4">
          <UFormGroup label="发布候选ID" name="releaseCandidateId" required>
            <UInput
              v-model="formState.releaseCandidateId"
              placeholder="输入 Release Candidate UUID"
            />
          </UFormGroup>

          <UFormGroup label="包体 URI" name="packageUri">
            <UInput
              v-model="formState.packageUri"
              placeholder="s3://bucket/path/to/package.pxp"
            />
          </UFormGroup>

          <UFormGroup label="校验和 (SHA256)" name="checksum" required>
            <UInput
              v-model="formState.checksum"
              placeholder="输入包体 SHA256 校验和"
            />
          </UFormGroup>

          <UFormGroup label="签名指纹" name="signatureFingerprint">
            <UInput
              v-model="formState.signatureFingerprint"
              placeholder="输入签名指纹"
            />
          </UFormGroup>

          <UFormGroup label="依赖列表" name="dependencies">
            <UTextarea
              v-model="dependenciesText"
              placeholder="每行一个依赖项"
              :rows="3"
            />
          </UFormGroup>

          <UFormGroup label="许可证报告" name="licenseReport">
            <UTextarea
              v-model="licenseReportText"
              placeholder="JSON 格式的许可证信息"
              :rows="3"
            />
          </UFormGroup>

          <div class="flex items-center gap-3">
            <UButton
              type="submit"
              color="primary"
              :loading="isSubmitting"
              :disabled="!formState.releaseCandidateId || !formState.checksum"
            >
              提交审核
            </UButton>
            <UButton @click="resetForm" variant="soft">重置</UButton>
          </div>
        </div>
      </UForm>
    </UCard>

    <!-- 提交结果 -->
    <UCard v-if="submitResult" class="mt-6">
      <template #header>
        <div class="flex items-center gap-2">
          <UIcon name="i-heroicons-check-circle" class="text-green-500" />
          <h3 class="text-lg font-semibold">提交成功</h3>
        </div>
      </template>

      <div class="space-y-3">
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="text-sm text-gray-500">离线包ID</label>
            <p class="font-mono">{{ submitResult.id }}</p>
          </div>
          <div>
            <label class="text-sm text-gray-500">状态</label>
            <p>{{ submitResult.status }}</p>
          </div>
          <div class="col-span-2">
            <label class="text-sm text-gray-500">校验和</label>
            <p class="font-mono text-sm">{{ submitResult.checksum }}</p>
          </div>
        </div>
        <div v-if="submitResult.auditId" class="mt-4 p-3 bg-blue-50 rounded">
          <p class="text-sm">
            <UIcon name="i-heroicons-information-circle" />
            审计参考: <code>{{ submitResult.auditId }}</code>
          </p>
        </div>
      </div>
    </UCard>

    <!-- 历史记录 -->
    <UCard class="mt-6">
      <template #header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">历史记录</h3>
          <UButton @click="loadPackages" variant="soft" size="sm">
            <UIcon name="i-heroicons-arrow-path" />
            刷新
          </UButton>
        </div>
      </template>

      <div v-if="isLoading" class="flex justify-center py-8">
        <USpinner size="lg" />
      </div>

      <UTable
        v-else-if="packages.length > 0"
        :columns="tableColumns"
        :rows="packages"
      >
        <template #status-data="{ row }">
          <UBadge :color="getStatusColor(row.status)">
            {{ row.status }}
          </UBadge>
        </template>
        <template #createdAt-data="{ row }">
          {{ formatDate(row.createdAt) }}
        </template>
      </UTable>

      <div v-else class="text-center py-8 text-gray-500">
        暂无离线包记录
      </div>
    </UCard>
  </div>
</template>

<script setup lang="ts">
import { useToast } from '#app'

// 页面元信息
definePageMeta({
  layout: 'admin',
  middleware: 'admin-only'
})

// Composables
const toast = useToast()

// 状态管理
const formState = reactive({
  releaseCandidateId: '',
  packageUri: '',
  checksum: '',
  signatureFingerprint: '',
  dependencies: [] as string[],
  licenseReport: {} as Record<string, any>
})

const dependenciesText = computed({
  get: () => formState.dependencies.join('\n'),
  set: (val: string) => {
    formState.dependencies = val.split('\n').filter(d => d.trim())
  }
})

const licenseReportText = computed({
  get: () => JSON.stringify(formState.licenseReport, null, 2),
  set: (val: string) => {
    try {
      formState.licenseReport = JSON.parse(val || '{}')
    } catch (e) {
      // 忽略JSON解析错误
    }
  }
})

// 本地状态
const isSubmitting = ref(false)
const isLoading = ref(false)
const submitResult = ref<any>(null)
const packages = ref<any[]>([])

// 表格列定义
const tableColumns = [
  { key: 'id', label: 'ID' },
  { key: 'releaseCandidateId', label: '候选ID' },
  { key: 'status', label: '状态' },
  { key: 'createdAt', label: '创建时间' }
]

// 方法
const resetForm = () => {
  formState.releaseCandidateId = ''
  formState.packageUri = ''
  formState.checksum = ''
  formState.signatureFingerprint = ''
  formState.dependencies = []
  formState.licenseReport = {}
  submitResult.value = null
}

const handleSubmit = async () => {
  isSubmitting.value = true
  try {
    // TODO: 调用 API
    // const result = await $fetch('/api/admin/plugin-release/offline-packages', {
    //   method: 'POST',
    //   headers: { Authorization: `Bearer ${useCookie('token').value}` },
    //   body: formState
    // })

    // 模拟API调用
    await new Promise(resolve => setTimeout(resolve, 1000))

    submitResult.value = {
      id: Math.floor(Math.random() * 10000),
      status: 'pending',
      checksum: formState.checksum,
      auditId: `audit-${Date.now()}`
    }

    toast.add({
      title: '提交成功',
      description: '离线包已提交审核',
      color: 'green'
    })

    // 重新加载列表
    loadPackages()
  } catch (error: any) {
    console.error('提交失败:', error)
    toast.add({
      title: '提交失败',
      description: error.message || '请检查输入信息',
      color: 'red'
    })
  } finally {
    isSubmitting.value = false
  }
}

const loadPackages = async () => {
  isLoading.value = true
  try {
    // TODO: 调用 API 获取历史记录
    // const data = await $fetch('/api/admin/plugin-release/offline-packages/history')

    // 模拟数据
    await new Promise(resolve => setTimeout(resolve, 500))
    packages.value = []
  } catch (error: any) {
    console.error('加载失败:', error)
    toast.add({
      title: '加载失败',
      description: error.message || '无法获取历史记录',
      color: 'red'
    })
  } finally {
    isLoading.value = false
  }
}

const getStatusColor = (status: string) => {
  const colorMap: Record<string, string> = {
    pending: 'yellow',
    approved: 'green',
    rejected: 'red'
  }
  return colorMap[status] || 'gray'
}

const formatDate = (date: string | Date) => {
  return new Date(date).toLocaleString('zh-CN')
}

// 页面加载时获取数据
onMounted(() => {
  loadPackages()
})
</script>
