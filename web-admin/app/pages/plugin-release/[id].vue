<template>
  <div class="space-y-4 p-4">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold">{{ t('pluginRelease.detailTitle', `发布候选 ${id}`) }}</h1>
        <p class="text-sm text-gray-500">{{ t('pluginRelease.detailSubtitle', '查看元数据与当前状态') }}</p>
      </div>
      <UButton variant="ghost" icon="i-heroicons-arrow-left" @click="navigateTo('/plugin-release')">
        {{ t('common.backToList', '返回列表') }}
      </UButton>
    </div>

    <UCard :loading="loading">
      <template #header>
        <div class="flex items-center justify-between">
          <div class="text-sm text-gray-600">
            {{ detail.pluginId || '-' }} · {{ detail.version || '-' }}
          </div>
          <div class="flex gap-2">
            <UBadge color="blue" variant="soft">{{ t('pluginRelease.approval', '审批') }}: {{ detail.approvalStatus || '-' }}</UBadge>
            <UBadge color="green" variant="soft">{{ t('pluginRelease.gate', '门禁') }}: {{ detail.gateStatus || '-' }}</UBadge>
          </div>
        </div>
      </template>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
        <div class="space-y-1">
          <div class="text-gray-500">{{ t('pluginRelease.pluginVersion', '插件/版本') }}</div>
          <div class="font-medium">{{ detail.pluginId || '-' }}{{ detail.version ? ` · ${detail.version}` : '' }}</div>
        </div>
        <div class="space-y-1">
          <div class="text-gray-500">{{ t('pluginRelease.tenant', '租户') }}</div>
          <div class="font-medium">{{ detail.tenantId || '-' }}</div>
        </div>
        <div class="space-y-1">
          <div class="text-gray-500">{{ t('pluginRelease.commitHash', '提交哈希') }}</div>
          <div class="font-medium break-all">{{ detail.commitHash || '-' }}</div>
        </div>
        <div class="space-y-1">
          <div class="text-gray-500">{{ t('pluginRelease.artifact', '构建产物') }}</div>
          <div class="font-medium break-all">{{ detail.buildArtifactUri || '-' }}</div>
        </div>
        <div class="space-y-1">
          <div class="text-gray-500">{{ t('pluginRelease.submittedAt', '提交时间') }}</div>
          <div class="font-medium">{{ detail.submittedAt || detail.createdAt || '-' }}</div>
        </div>
      <div class="space-y-1">
        <div class="text-gray-500">{{ t('pluginRelease.createdBy', '提交人') }}</div>
        <div class="font-medium">{{ detail.createdByDisplay || '-' }}</div>
      </div>
        <div class="space-y-1 md:col-span-2">
          <div class="text-gray-500">{{ t('pluginRelease.labels', '标签') }}</div>
          <div class="flex flex-wrap gap-2">
            <template v-if="labelEntries.length">
              <UBadge v-for="(item, idx) in labelEntries" :key="idx" variant="soft">
                {{ item[0] }}: {{ item[1] }}
              </UBadge>
            </template>
            <span v-else class="text-gray-400">-</span>
          </div>
        </div>
        <div class="space-y-1 md:col-span-2">
          <div class="text-gray-500">{{ t('pluginRelease.releaseNotes', '发布说明') }}</div>
          <div class="font-medium whitespace-pre-wrap">{{ detail.releaseNotes || '-' }}</div>
        </div>
      </div>
    </UCard>
  </div>
</template>

<script setup lang="ts">
import pluginReleaseService from '~/composables/api/services/pluginRelease'

definePageMeta({
  layout: 'default'
})

const { t } = useI18n()
const route = useRoute()
const id = computed(() => String(route.params.id || ''))
const loading = ref(false)
const detail = reactive<any>({
  pluginId: '',
  version: '',
  tenantId: '',
  releaseNotes: '',
  buildArtifactUri: '',
  commitHash: '',
  approvalStatus: '',
  gateStatus: '',
  createdBy: '',
  createdByDisplay: '',
  submittedAt: '',
  createdAt: '',
  labels: {}
})

// 将 JWT/字符串转换为可读身份标识
function decodeBase64Url(input: string): string {
  // JWT payload 是 base64url，需要替换并补齐 padding
  const normalized = input.replace(/-/g, '+').replace(/_/g, '/')
  const padLength = (4 - (normalized.length % 4)) % 4
  return atob(normalized + '='.repeat(padLength))
}

function getCreatedByDisplay(raw?: string, log = false): string {
  const value = raw || ''
  if (!value) {
    if (log) console.info('[plugin-release] createdBy empty')
    return '-'
  }
  const parts = value.split('.')
  if (log) console.info('[plugin-release] createdBy raw', { raw: value, parts: parts.length })
  if (parts.length >= 2) {
    try {
      const payload = JSON.parse(decodeBase64Url(parts[1]))
      const display =
        payload.name ||
        payload.email ||
        payload.username ||
        payload.sub ||
        payload.tid ||
        value
      if (log) {
        console.info('[plugin-release] createdBy decoded', { raw: value, payload, display })
      }
      return display
    } catch (err) {
      // 尝试宽松提取 (支持缺失花括号/结尾逗号的 payload 文本)
      try {
        const payloadText = decodeBase64Url(parts[1])
        if (log) console.warn('[plugin-release] createdBy decode JSON failed, scanning text', { raw: value, payloadText, err })
        // 尝试修复尾逗号/缺少右括号
        let sanitized = payloadText.replace(/,?\s*$/, '')
        if (!sanitized.trim().endsWith('}')) sanitized += '}'
        try {
          const loose = JSON.parse(sanitized)
          const display =
            loose.name ||
            loose.email ||
            loose.username ||
            loose.sub ||
            loose.tid
          if (display) {
            if (log) console.info('[plugin-release] createdBy loose-decoded', { raw: value, sanitized, display })
            return display
          }
        } catch (_looseErr) {
          // ignore, fallback regex below
        }
        const m =
          payloadText.match(/"name"\s*:\s*"([^"}]+)/) ||
          payloadText.match(/"email"\s*:\s*"([^"}]+)/) ||
          payloadText.match(/"username"\s*:\s*"([^"}]+)/) ||
          payloadText.match(/"sub"\s*:\s*"([^"}]+)/) ||
          payloadText.match(/"tid"\s*:\s*"?(?<tid>[A-Za-z0-9-]+)/) ||
          payloadText.match(/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}/)
        if (log) {
          console.warn('[plugin-release] createdBy decode failed, fallback text scan', {
            raw: value,
            payloadText,
            error: err,
            match: m?.[1],
          })
        }
        const matched =
          (m?.groups && (m.groups as any).tid) ||
          (Array.isArray(m) ? m[1] : undefined)
        if (matched) return matched
      } catch (inner) {
        if (log) {
          console.warn('[plugin-release] createdBy decode & scan failed', value, inner)
        }
      }
      // 最终回退：返回原值（前缀过的 sub:/tok: 等），避免显示系统
      return value
    }
  }
  if (log) {
    console.info('[plugin-release] createdBy plain', value)
  }
  return value
}

const labelEntries = computed(() => Object.entries(detail.labels || {}))

async function fetchDetail() {
  if (!id.value) return
  loading.value = true
  try {
    const data = await pluginReleaseService.getReleaseCandidate(id.value)
    Object.assign(detail, data || {})
    detail.createdByDisplay = getCreatedByDisplay(detail.createdBy, true)
  } catch (e) {
    console.error('加载候选详情失败', e)
  } finally {
    loading.value = false
  }
}

onMounted(fetchDetail)
</script>
