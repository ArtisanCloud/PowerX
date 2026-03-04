<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui'
import { computed } from 'vue'
import { useI18n } from '#imports'
import { usePluginBridge } from '~/composables/usePluginBridge'

const { locale, locales, setLocale } = useI18n()
const switchLocalePath = useSwitchLocalePath()
const route = useRoute()
const { broadcast } = usePluginBridge()

// 显示用国旗
const flags = { zh: '🇨🇳', en: '🇺🇸', ja: '🇯🇵', ko: '🇰🇷' } as const

type LocaleEntry = string | { code: string; name?: string; iso?: string; [k: string]: any }
function toCode(l: LocaleEntry): string {
  if (typeof l === 'string') return l
  if (l && typeof l === 'object') return l.code || l.iso || ''
  return ''
}
function toName(l: LocaleEntry): string {
  if (typeof l === 'string') return l
  if (l && typeof l === 'object') return l.name || l.code || l.iso || ''
  return ''
}

const normalized = computed(() => {
  const arr = (locales?.value as LocaleEntry[] | undefined) || []
  return arr.map(l => {
    const code = toCode(l)
    const key = code.split('-')[0] as keyof typeof flags
    return {
      code,
      name: toName(l),
      flag: flags[key] ?? '🏳️',
    }
  })
})

const current = computed(() => {
  const code = String(locale?.value ?? '')
  const hit = normalized.value.find(x => x.code === code)
  if (hit) return hit
  // 兜底：第一个或默认 zh
  return normalized.value[0] ?? { code: 'zh', name: '简体中文', flag: flags.zh }
})

async function select(code: string) {
  // 优先使用 i18n 路由切换
  const path = switchLocalePath(code)
  await setLocale(code)
  if (path && path !== route.fullPath) {
    await navigateTo(path)
  }
  // 只广播字符串 code
  broadcast({ source: 'powerx', type: 'locale', locale: code })
}

// DropdownMenu items
const items = computed<DropdownMenuItem[][]>(() => [
  normalized.value.map((opt) => ({
    label: `${opt.flag} ${opt.name}`,
    icon: undefined,
    onSelect: () => select(opt.code),
  }))
])
</script>

<template>
  <UDropdownMenu :items="items">
    <UButton variant="ghost" class="flex items-center gap-2">
      <span class="text-lg">{{ current?.flag ?? '🏳️' }}</span>
      <span class="hidden sm:inline">{{ current?.name ?? 'Language' }}</span>
      <UIcon class="w-4 h-4 inline-block" name="i-heroicons-chevron-down-20-solid" />
    </UButton>
  </UDropdownMenu>
</template>
