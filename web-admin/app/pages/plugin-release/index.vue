<template>
  <div class="space-y-6 p-4">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold">发布候选</h1>
        <p class="text-sm text-gray-500">查看已登记的版本，跳转到详情或补件</p>
      </div>
      <UButton icon="i-heroicons-arrow-path" size="sm" @click="fetchList" :loading="loading">
        刷新
      </UButton>
    </div>

    <div class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4 space-y-3">
      <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
        <UInput v-model="filters.pluginId" placeholder="插件ID" />
        <UInput v-model="filters.version" placeholder="版本前缀" />
        <UInput v-model="filters.tenantId" placeholder="租户ID" />
      </div>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
        <USelect v-model="filters.approvalStatus" :items="statusOptions" placeholder="审批状态" />
        <USelect v-model="filters.gateStatus" :items="gateOptions" placeholder="门禁状态" />
        <div class="flex gap-2">
          <UButton size="sm" @click="applyFilters">应用筛选</UButton>
          <UButton size="sm" variant="ghost" @click="resetFilters">重置</UButton>
        </div>
      </div>
    </div>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <span class="text-sm text-gray-600">共 {{ pagination.total }} 条</span>
          <span class="text-xs text-gray-400">第 {{ pagination.page }} 页</span>
        </div>
      </template>
      <UTable :columns="columns" :data="rows" :loading="loading" row-key="candidateId" />
      <template #footer>
        <div class="flex justify-center">
          <UPagination v-model:page="pagination.page" :total="pagination.total" :items-per-page="pagination.pageSize" />
        </div>
      </template>
    </UCard>
  </div>
</template>

<script setup lang="ts">
import { h, resolveComponent } from 'vue'
import { useToast, useOverlay } from '#imports'
import { LazyPluginReleaseEditCandidateModal, LazyCommonConfirmModal } from '#components'
import { useOneShotAlert } from '~/composables/useOneShotAlert'
import pluginReleaseService, { type ReleaseCandidateSummary } from '~/composables/api/services/pluginRelease'

definePageMeta({
  layout: 'default'
})

const loading = ref(false)
const toast = useToast()
const oneShot = useOneShotAlert()
const overlay = useOverlay()
const editModal = overlay.create(LazyPluginReleaseEditCandidateModal)
const confirmModal = overlay.create(LazyCommonConfirmModal)
const filters = reactive({
  pluginId: '',
  tenantId: '',
  version: '',
  approvalStatus: '',
  gateStatus: ''
})
const statusOptions = [
  { label: '全部审批状态', value: 'all' },
  { label: 'draft', value: 'draft' },
  { label: 'submitted', value: 'submitted' },
  { label: 'approved', value: 'approved' },
  { label: 'rejected', value: 'rejected' }
]
const gateOptions = [
  { label: '全部门禁状态', value: 'all' },
  { label: 'pending', value: 'pending' },
  { label: 'passed', value: 'passed' },
  { label: 'failed', value: 'failed' }
]

const pagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0
})

const rows = ref<ReleaseCandidateSummary[]>([])

const statusColorMap: Record<string, string> = {
  draft: 'gray',
  submitted: 'blue',
  approved: 'green',
  rejected: 'red',
  pending: 'amber',
  passed: 'green',
  failed: 'red',
}

const columns = [
  {
    accessorKey: 'pluginId',
    header: '插件/版本',
    cell: ({ row }: any) =>
      h('div', { class: 'space-y-1' }, [
        h('div', { class: 'font-semibold text-sm' }, row.original.pluginId || '-'),
        h('div', { class: 'text-xs text-gray-500' }, row.original.version || '-'),
      ]),
  },
  {
    accessorKey: 'tenantId',
    header: '租户',
    cell: ({ row }: any) =>
      h('div', { class: 'text-sm text-gray-200' }, row.original.tenantId || '-'),
  },
  {
    accessorKey: 'approvalStatus',
    header: '审批',
    cell: ({ row }: any) =>
      h(resolveComponent('UBadge'), {
        color: statusColorMap[row.original.approvalStatus] || 'gray',
        variant: 'soft',
      }, () => row.original.approvalStatus || '-'),
  },
  {
    accessorKey: 'gateStatus',
    header: '门禁',
    cell: ({ row }: any) =>
      h(resolveComponent('UBadge'), {
        color: statusColorMap[row.original.gateStatus] || 'gray',
        variant: 'soft',
      }, () => row.original.gateStatus || '-'),
  },
  {
    accessorKey: 'offlinePackageStatus',
    header: '制品',
    cell: ({ row }: any) =>
      h('div', { class: 'text-sm text-gray-200' }, [
        h('div', `${row.original.offlinePackageCount || 0} 个`),
        h('div', { class: 'text-xs text-gray-500' }, row.original.offlinePackageStatus || '未上传'),
      ]),
  },
  {
    accessorKey: 'createdAt',
    header: '创建时间',
    cell: ({ row }: any) =>
      h('div', { class: 'text-xs text-gray-500' }, row.original.createdAt || '-'),
  },
  {
    id: 'actions',
    header: '操作',
    cell: ({ row }: any) =>
      h('div', { class: 'flex gap-2' }, [
        h(resolveComponent('UButton'), { size: 'xs', variant: 'ghost', onClick: () => goDetail(row.original.candidateId) }, () => '详情'),
        h(resolveComponent('UButton'), { size: 'xs', variant: 'soft', onClick: () => openMarket(row.original.pluginId) }, () => '市场'),
        h(resolveComponent('UButton'), { size: 'xs', color: 'error', variant: 'soft', onClick: () => confirmDelete(row.original) }, () => '删除'),
        h(resolveComponent('UButton'), { size: 'xs', variant: 'outline', onClick: () => openEdit(row.original) }, () => '更新'),
      ]),
  },
]

async function fetchList() {
  loading.value = true
  try {
    const res = await pluginReleaseService.listReleaseCandidates({
      page: pagination.page,
      size: pagination.pageSize,
      pluginId: filters.pluginId || undefined,
      tenantId: filters.tenantId || undefined,
      version: filters.version || undefined,
      approvalStatus: toFilterValue(filters.approvalStatus),
      gateStatus: toFilterValue(filters.gateStatus)
    })
    rows.value = res.items || []
    const p = res.pagination || {}
    pagination.total = p.total ?? rows.value.length
    pagination.page = p.page ?? pagination.page
    pagination.pageSize = p.pageSize || p.page_size || pagination.pageSize
  } catch (e) {
    console.error('加载发布候选失败', e)
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  pagination.page = 1
  fetchList()
}

function resetFilters() {
  filters.pluginId = ''
  filters.tenantId = ''
  filters.version = ''
  filters.approvalStatus = ''
  filters.gateStatus = ''
  applyFilters()
}

function toFilterValue(val: string) {
  if (!val || val === 'all') return undefined
  return val
}

function goDetail(id: string) {
  navigateTo(`/plugin-release/${id}`)
}

function openMarket(pluginId: string) {
  navigateTo(`/plugins/market?pluginId=${encodeURIComponent(pluginId)}`)
}

async function openEdit(row: ReleaseCandidateSummary) {
  if (!row?.candidateId) return
  try {
    const instance = editModal.open({
      candidateId: row.candidateId,
      pluginId: row.pluginId,
      version: row.version,
      buildArtifactUri: row.buildArtifactUri,
      releaseNotes: row.releaseNotes,
      labels: row.labels,
    })
    const payload = await instance.result
    if (!payload) return
    await pluginReleaseService.updateReleaseCandidate(row.candidateId, {
      buildArtifact: payload.buildArtifact,
      releaseNotes: payload.releaseNotes,
      labels: payload.labels,
    })
    toast.add({ title: '更新成功', color: 'success' })
    await fetchList()
  } catch (e) {
    console.error('更新候选失败', e)
    toast.add({ title: '更新失败', color: 'error' })
  }
}

async function confirmDelete(row: ReleaseCandidateSummary) {
  if (!row?.candidateId) return
  const modal = confirmModal.open({
    title: '删除候选',
    message: `确认删除候选 ${row.pluginId} ${row.version}？删除后不可恢复。`,
    tone: 'danger',
    confirmColor: 'danger',
    confirmLabel: '删除',
    cancelLabel: '取消',
  })
  const ok = await modal.result
  if (!ok) return
  loading.value = true
  try {
    await pluginReleaseService.deleteReleaseCandidate(row.candidateId)
    toast.add({ title: '删除成功', color: 'success' })
    oneShot.reset()
    oneShot.notifyOnce('删除成功', `${row.pluginId} ${row.version} 已删除`, 'error', 'soft')
    await fetchList()
  } catch (e) {
    console.error('删除候选失败', e)
    toast.add({ title: '删除失败', color: 'error' })
  } finally {
    loading.value = false
  }
}

watch(() => pagination.page, fetchList)

onMounted(fetchList)
</script>
