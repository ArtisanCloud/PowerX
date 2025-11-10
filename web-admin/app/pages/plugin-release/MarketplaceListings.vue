<template>
  <div class="marketplace-listings">
    <div class="page-header">
      <h1>Marketplace 审核列表</h1>
      <p class="text-gray-600">管理离线包审核状态与SLA</p>
    </div>

    <!-- 筛选器 -->
    <UCard class="mb-4">
      <div class="flex items-center gap-4">
        <UFormGroup label="状态">
          <USelectMenu
            v-model="filters.status"
            :options="statusOptions"
            placeholder="全部状态"
            @change="loadListings"
          />
        </UFormGroup>

        <UFormGroup label="渠道">
          <USelectMenu
            v-model="filters.channel"
            :options="channelOptions"
            placeholder="全部渠道"
            @change="loadListings"
          />
        </UFormGroup>

        <div class="flex-1" />

        <UButton @click="loadListings" variant="soft">
          <UIcon name="i-heroicons-arrow-path" />
          刷新
        </UButton>
      </div>
    </UCard>

    <!-- 列表 -->
    <UCard>
      <div v-if="isLoading" class="flex justify-center py-12">
        <USpinner size="lg" />
      </div>

      <UTable
        v-else-if="listings.length > 0"
        :columns="tableColumns"
        :rows="listings"
        :loading="isLoading"
      >
        <template #status-data="{ row }">
          <UBadge :color="getStatusColor(row.reviewStatus)">
            {{ row.reviewStatus }}
          </UBadge>
        </template>

        <template #reviewCount-data="{ row }">
          <div class="flex items-center gap-2">
            <span>{{ row.reviewCount }}</span>
            <UButton
              v-if="row.reviewCount > 0"
              @click="viewDetails(row.id)"
              variant="soft"
              size="xs"
            >
              查看
            </UButton>
          </div>
        </template>

        <template #slaCountdown-data="{ row }">
          <div v-if="row.slaDeadline" class="text-sm">
            <span :class="getSlaClass(row.slaDeadline)">
              {{ formatSlaCountdown(row.slaDeadline) }}
            </span>
          </div>
          <span v-else class="text-gray-400">-</span>
        </template>

        <template #actions-data="{ row }">
          <div class="flex gap-2">
            <UButton
              @click="viewDetails(row.id)"
              variant="soft"
              size="sm"
            >
              详情
            </UButton>
            <UButton
              v-if="row.reviewStatus === 'pending'"
              @click="openReviewModal(row)"
              color="primary"
              size="sm"
            >
              审核
            </UButton>
          </div>
        </template>
      </UTable>

      <div v-else class="text-center py-12 text-gray-500">
        暂无审核记录
      </div>

      <!-- 分页 -->
      <div v-if="pagination.total > 0" class="flex items-center justify-between mt-4">
        <div class="text-sm text-gray-500">
          共 {{ pagination.total }} 条记录，第 {{ pagination.page }}/{{ Math.ceil(pagination.total / pagination.size) }} 页
        </div>
        <UPagination
          v-model="pagination.page"
          :page-count="Math.ceil(pagination.total / pagination.size)"
          :total="pagination.total"
          @update:model-value="loadListings"
        />
      </div>
    </UCard>

    <!-- 审核操作模态框 -->
    <UModal v-model="isReviewModalOpen">
      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <h3 class="text-lg font-semibold">审核操作</h3>
            <UButton
              @click="isReviewModalOpen = false"
              variant="soft"
              icon="i-heroicons-x-mark"
            />
          </div>
        </template>

        <div v-if="selectedListing" class="space-y-4">
          <div class="p-4 bg-gray-50 rounded">
            <h4 class="font-semibold mb-2">Listing #{{ selectedListing.id }}</h4>
            <div class="grid grid-cols-2 gap-2 text-sm">
              <div>
                <span class="text-gray-500">渠道:</span>
                <span class="ml-2">{{ selectedListing.channel }}</span>
              </div>
              <div>
                <span class="text-gray-500">审核次数:</span>
                <span class="ml-2">{{ selectedListing.reviewCount }}</span>
              </div>
            </div>
          </div>

          <UForm :state="reviewForm" @submit="handleReview">
            <UFormGroup label="审核结果" name="decision" required>
              <USelectMenu
                v-model="reviewForm.decision"
                :options="reviewOptions"
                placeholder="选择审核结果"
              />
            </UFormGroup>

            <UFormGroup label="审核意见" name="comment">
              <UTextarea
                v-model="reviewForm.comment"
                placeholder="请输入审核意见..."
                :rows="3"
              />
            </UFormGroup>

            <div class="flex justify-end gap-3">
              <UButton
                @click="isReviewModalOpen = false"
                variant="soft"
              >
                取消
              </UButton>
              <UButton
                type="submit"
                color="primary"
                :loading="isReviewing"
                :disabled="!reviewForm.decision"
              >
                提交审核
              </UButton>
            </div>
          </UForm>
        </div>
      </UCard>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { useToast } from '#imports'

// 页面元信息
definePageMeta({
  layout: 'admin',
  middleware: 'admin-only'
})

// Composables
const toast = useToast()
const route = useRoute()
const router = useRouter()

// 状态
const isLoading = ref(false)
const listings = ref<any[]>([])
const isReviewModalOpen = ref(false)
const isReviewing = ref(false)
const selectedListing = ref<any>(null)

const filters = reactive({
  status: '',
  channel: ''
})

const reviewForm = reactive({
  decision: '',
  comment: ''
})

const pagination = reactive({
  page: 1,
  size: 20,
  total: 0
})

// 选项
const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '待审核', value: 'pending' },
  { label: '需补件', value: 'need_fix' },
  { label: '已通过', value: 'approved' },
  { label: '已拒绝', value: 'rejected' }
]

const channelOptions = [
  { label: '全部渠道', value: '' },
  { label: '在线', value: 'online' },
  { label: '离线', value: 'offline' }
]

const reviewOptions = [
  { label: '通过', value: 'approved' },
  { label: '拒绝', value: 'rejected' },
  { label: '需补件', value: 'need_fix' }
]

// 表格列
const tableColumns = [
  { key: 'id', label: 'ID' },
  { key: 'offlinePackageId', label: '离线包ID' },
  { key: 'channel', label: '渠道' },
  { key: 'status', label: '状态' },
  { key: 'reviewCount', label: '审核次数' },
  { key: 'slaCountdown', label: 'SLA倒计时' },
  { key: 'actions', label: '操作' }
]

// 方法
const loadListings = async () => {
  isLoading.value = true
  try {
    const params = {
      page: pagination.page,
      size: pagination.size,
      ...(filters.status && { status: filters.status }),
      ...(filters.channel && { channel: filters.channel })
    }

    // TODO: 调用API
    // const data = await $fetch('/api/admin/plugin-release/marketplace/listings', {
    //   method: 'GET',
    //   headers: { Authorization: `Bearer ${useCookie('token').value}` },
    //   query: params
    // })

    // 模拟数据
    await new Promise(resolve => setTimeout(resolve, 800))
    listings.value = []
    pagination.total = 0

    toast.add({
      title: '加载成功',
      description: `获取到 ${listings.value.length} 条记录`,
      color: 'green'
    })
  } catch (error: any) {
    console.error('加载失败:', error)
    toast.add({
      title: '加载失败',
      description: error.message || '无法获取审核列表',
      color: 'red'
    })
  } finally {
    isLoading.value = false
  }
}

const viewDetails = (id: number) => {
  router.push(`/admin/plugin-release/marketplace/${id}`)
}

const openReviewModal = (listing: any) => {
  selectedListing.value = listing
  reviewForm.decision = ''
  reviewForm.comment = ''
  isReviewModalOpen.value = true
}

const handleReview = async () => {
  if (!selectedListing.value || !reviewForm.decision) return

  isReviewing.value = true
  try {
    // TODO: 调用API
    // await $fetch(`/api/admin/plugin-release/marketplace/listings/${selectedListing.value.id}/reviews`, {
    //   method: 'POST',
    //   headers: { Authorization: `Bearer ${useCookie('token').value}` },
    //   body: reviewForm
    // })

    await new Promise(resolve => setTimeout(resolve, 1000))

    toast.add({
      title: '审核成功',
      description: `已${getReviewText(reviewForm.decision)}该申请`,
      color: 'green'
    })

    isReviewModalOpen.value = false
    loadListings()
  } catch (error: any) {
    console.error('审核失败:', error)
    toast.add({
      title: '审核失败',
      description: error.message || '请重试',
      color: 'red'
    })
  } finally {
    isReviewing.value = false
  }
}

const getStatusColor = (status: string) => {
  const colorMap: Record<string, string> = {
    pending: 'yellow',
    need_fix: 'orange',
    approved: 'green',
    rejected: 'red',
    published: 'blue'
  }
  return colorMap[status] || 'gray'
}

const getSlaClass = (deadline: string) => {
  const now = new Date().getTime()
  const deadlineTime = new Date(deadline).getTime()
  const hoursLeft = (deadlineTime - now) / (1000 * 60 * 60)

  if (hoursLeft < 24) return 'text-red-500 font-semibold'
  if (hoursLeft < 48) return 'text-orange-500'
  return 'text-gray-600'
}

const formatSlaCountdown = (deadline: string) => {
  const now = new Date().getTime()
  const deadlineTime = new Date(deadline).getTime()
  const hoursLeft = Math.floor((deadlineTime - now) / (1000 * 60 * 60))

  if (hoursLeft < 0) return '已超时'
  if (hoursLeft < 24) return `${hoursLeft}小时后超时`
  const daysLeft = Math.floor(hoursLeft / 24)
  return `${daysLeft}天${hoursLeft % 24}小时后超时`
}

const getReviewText = (decision: string) => {
  const textMap: Record<string, string> = {
    approved: '通过',
    rejected: '拒绝',
    need_fix: '要求补件'
  }
  return textMap[decision] || decision
}

// 监听路由参数变化
watch(() => route.query, () => {
  if (route.query.page) {
    pagination.page = parseInt(route.query.page as string)
  }
  loadListings()
})

// 初始化
onMounted(() => {
  if (route.query.page) {
    pagination.page = parseInt(route.query.page as string)
  }
  loadListings()
})
</script>
