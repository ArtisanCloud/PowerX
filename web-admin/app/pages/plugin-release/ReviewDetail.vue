<template>
  <div class="review-detail">
    <div class="page-header">
      <div class="flex items-center gap-4">
        <UButton
          @click="goBack"
          variant="soft"
          icon="i-heroicons-arrow-left"
        />
        <div>
          <h1>审核详情 #{{ listingId }}</h1>
          <p class="text-gray-600">查看和操作 Marketplace Listing</p>
        </div>
      </div>
      <div v-if="listing" class="flex items-center gap-2">
        <UBadge :color="getStatusColor(listing.reviewStatus)">
          {{ getStatusText(listing.reviewStatus) }}
        </UBadge>
      </div>
    </div>

    <div v-if="isLoading" class="flex justify-center py-12">
      <USpinner size="lg" />
    </div>

    <div v-else-if="listing" class="space-y-6">
      <!-- 基本信息 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">基本信息</h3>
        </template>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="text-sm text-gray-500">ID</label>
            <p class="font-mono">{{ listing.id }}</p>
          </div>
          <div>
            <label class="text-sm text-gray-500">离线包ID</label>
            <p class="font-mono">{{ listing.offlinePackageId }}</p>
          </div>
          <div>
            <label class="text-sm text-gray-500">渠道</label>
            <p>{{ listing.channel }}</p>
          </div>
          <div>
            <label class="text-sm text-gray-500">审核次数</label>
            <p>{{ listing.reviewCount }}</p>
          </div>
          <div>
            <label class="text-sm text-gray-500">创建时间</label>
            <p>{{ formatDate(listing.createdAt) }}</p>
          </div>
          <div v-if="listing.publishedAt">
            <label class="text-sm text-gray-500">发布时间</label>
            <p>{{ formatDate(listing.publishedAt) }}</p>
          </div>
        </div>
      </UCard>

      <!-- 定价信息 -->
      <UCard v-if="listing.pricing">
        <template #header>
          <h3 class="text-lg font-semibold">定价策略</h3>
        </template>

        <div class="space-y-2">
          <div v-for="(value, key) in listing.pricing" :key="key" class="flex gap-4">
            <span class="text-gray-500 w-32">{{ key }}:</span>
            <span>{{ String(value) }}</span>
          </div>
        </div>
      </UCard>

      <!-- 支持策略 -->
      <UCard v-if="listing.supportPolicy">
        <template #header>
          <h3 class="text-lg font-semibold">支持策略</h3>
        </template>

        <div class="space-y-2">
          <div v-for="(value, key) in listing.supportPolicy" :key="key" class="flex gap-4">
            <span class="text-gray-500 w-32">{{ key }}:</span>
            <span>{{ String(value) }}</span>
          </div>
        </div>
      </UCard>

      <!-- 审核历史 -->
      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <h3 class="text-lg font-semibold">审核历史</h3>
            <UButton
              v-if="listing.reviewStatus === 'pending'"
              @click="openReviewModal"
              color="primary"
              size="sm"
            >
              进行审核
            </UButton>
          </div>
        </template>

        <div v-if="reviewHistory.length > 0" class="space-y-4">
          <div
            v-for="review in reviewHistory"
            :key="review.id"
            class="p-4 border rounded"
          >
            <div class="flex items-start justify-between mb-2">
              <div>
                <UBadge :color="getStatusColor(review.decision)" size="sm">
                  {{ getStatusText(review.decision) }}
                </UBadge>
                <span class="ml-2 text-sm text-gray-500">
                  {{ formatDate(review.createdAt) }}
                </span>
              </div>
              <span class="text-sm text-gray-500">
                审核员: {{ review.actor }}
              </span>
            </div>
            <p v-if="review.comment" class="text-sm text-gray-700">
              {{ review.comment }}
            </p>
          </div>
        </div>

        <div v-else class="text-center py-8 text-gray-500">
          暂无审核历史
        </div>
      </UCard>

      <!-- SLA信息 -->
      <UCard>
        <template #header>
          <h3 class="text-lg font-semibold">SLA 监控</h3>
        </template>

        <div v-if="listing.slaDeadline" class="space-y-3">
          <div class="flex items-center gap-4">
            <UIcon
              :name="getSlaIcon()"
              :class="getSlaClass()"
            />
            <div>
              <p class="font-semibold">
                截止时间: {{ formatDate(listing.slaDeadline) }}
              </p>
              <p :class="getSlaClass()">
                {{ getSlaText() }}
              </p>
            </div>
          </div>

          <div v-if="isSlaUrgent" class="p-3 bg-red-50 border border-red-200 rounded">
            <div class="flex items-center gap-2">
              <UIcon name="i-heroicons-exclamation-triangle" class="text-red-500" />
              <p class="text-sm text-red-700 font-semibold">
                此申请即将超时，请优先处理！
              </p>
            </div>
          </div>
        </div>

        <div v-else class="text-gray-500">
          未设置SLA截止时间
        </div>
      </UCard>
    </div>

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

        <UForm :state="reviewForm" @submit="handleReview">
          <div class="space-y-4">
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
                placeholder="请详细说明审核意见，特别是拒绝或需要补件的原因..."
                :rows="4"
              />
            </UFormGroup>

            <div class="p-4 bg-blue-50 rounded">
              <div class="flex items-start gap-3">
                <UIcon name="i-heroicons-information-circle" class="text-blue-500 mt-0.5" />
                <div class="text-sm text-blue-700">
                  <p class="font-semibold mb-1">审核说明：</p>
                  <ul class="list-disc list-inside space-y-1">
                    <li>通过：申请通过审核，将在5分钟内上架到Marketplace</li>
                    <li>拒绝：申请被拒绝，提交者需重新提交</li>
                    <li>需补件：要求提交者补充材料，审核计数+1</li>
                  </ul>
                </div>
              </div>
            </div>

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
          </div>
        </UForm>
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
const listingId = computed(() => route.params.id as string)
const isLoading = ref(true)
const listing = ref<any>(null)
const reviewHistory = ref<any[]>([])
const isReviewModalOpen = ref(false)
const isReviewing = ref(false)

const reviewForm = reactive({
  decision: '',
  comment: ''
})

// 选项
const reviewOptions = [
  { label: '通过', value: 'approved' },
  { label: '拒绝', value: 'rejected' },
  { label: '需补件', value: 'need_fix' }
]

// 计算属性
const isSlaUrgent = computed(() => {
  if (!listing.value?.slaDeadline) return false
  const now = new Date().getTime()
  const deadlineTime = new Date(listing.value.slaDeadline).getTime()
  return deadlineTime - now < 24 * 60 * 60 * 1000 // 24小时内
})

// 方法
const loadListing = async () => {
  isLoading.value = true
  try {
    // TODO: 调用API获取详情
    // const data = await $fetch(`/api/admin/plugin-release/marketplace/listings/${listingId.value}`, {
    //   headers: { Authorization: `Bearer ${useCookie('token').value}` }
    // })

    // 模拟数据
    await new Promise(resolve => setTimeout(resolve, 500))
    listing.value = {
      id: listingId.value,
      offlinePackageId: 123,
      channel: 'online',
      reviewStatus: 'pending',
      reviewCount: 0,
      createdAt: new Date(),
      slaDeadline: new Date(Date.now() + 48 * 60 * 60 * 1000),
      pricing: { tier: 'enterprise', price: 999 },
      supportPolicy: { sla: '24x7', responseTime: '1h' }
    }
    reviewHistory.value = []
  } catch (error: any) {
    console.error('加载失败:', error)
    toast.add({
      title: '加载失败',
      description: error.message || '无法获取详情',
      color: 'red'
    })
  } finally {
    isLoading.value = false
  }
}

const loadReviewHistory = async () => {
  // TODO: 获取审核历史
  // const history = await $fetch(`/api/admin/plugin-release/marketplace/listings/${listingId.value}/reviews`)
  // reviewHistory.value = history
}

const openReviewModal = () => {
  reviewForm.decision = ''
  reviewForm.comment = ''
  isReviewModalOpen.value = true
}

const handleReview = async () => {
  if (!reviewForm.decision) return

  isReviewing.value = true
  try {
    // TODO: 调用API
    // await $fetch(`/api/admin/plugin-release/marketplace/listings/${listingId.value}/reviews`, {
    //   method: 'POST',
    //   headers: { Authorization: `Bearer ${useCookie('token').value}` },
    //   body: reviewForm
    // })

    await new Promise(resolve => setTimeout(resolve, 1000))

    toast.add({
      title: '审核成功',
      description: `已${getStatusText(reviewForm.decision)}该申请`,
      color: 'green'
    })

    isReviewModalOpen.value = false
    loadListing()
    loadReviewHistory()
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

const goBack = () => {
  router.push('/admin/plugin-release/marketplace')
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

const getStatusText = (status: string) => {
  const textMap: Record<string, string> = {
    pending: '待审核',
    need_fix: '需补件',
    approved: '已通过',
    rejected: '已拒绝',
    published: '已上架'
  }
  return textMap[status] || status
}

const formatDate = (date: string | Date) => {
  return new Date(date).toLocaleString('zh-CN')
}

const getSlaClass = () => {
  if (!listing.value?.slaDeadline) return 'text-gray-600'
  const now = new Date().getTime()
  const deadlineTime = new Date(listing.value.slaDeadline).getTime()
  const hoursLeft = (deadlineTime - now) / (1000 * 60 * 60)

  if (hoursLeft < 24) return 'text-red-500 font-semibold'
  if (hoursLeft < 48) return 'text-orange-500'
  return 'text-gray-600'
}

const getSlaText = () => {
  if (!listing.value?.slaDeadline) return ''
  const now = new Date().getTime()
  const deadlineTime = new Date(listing.value.slaDeadline).getTime()
  const hoursLeft = Math.floor((deadlineTime - now) / (1000 * 60 * 60))

  if (hoursLeft < 0) return '已超时！'
  if (hoursLeft < 24) return `剩余 ${hoursLeft} 小时`
  const daysLeft = Math.floor(hoursLeft / 24)
  return `剩余 ${daysLeft} 天 ${hoursLeft % 24} 小时`
}

const getSlaIcon = () => {
  if (!listing.value?.slaDeadline) return 'i-heroicons-clock'
  const now = new Date().getTime()
  const deadlineTime = new Date(listing.value.slaDeadline).getTime()
  const hoursLeft = (deadlineTime - now) / (1000 * 60 * 60)

  if (hoursLeft < 24) return 'i-heroicons-exclamation-triangle text-red-500'
  if (hoursLeft < 48) return 'i-heroicons-clock text-orange-500'
  return 'i-heroicons-check-circle text-green-500'
}

// 初始化
onMounted(() => {
  loadListing()
  loadReviewHistory()
})
</script>
