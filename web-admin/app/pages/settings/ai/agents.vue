<template>
  <div class="space-y-6 p-4">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">智能体管理</h1>
        <p class="text-sm text-[var(--text-secondary)]">
          统一管理单智能体，支持快速进入智能会话或创建团队。
        </p>
      </div>
      <div class="flex items-center gap-2">
        <UButton variant="soft" icon="i-heroicons-chat-bubble-left-right" :to="localePath('/agent/sessions')">
          进入智能会话
        </UButton>
        <UButton color="primary" icon="i-heroicons-plus" @click="createAgentQuick">
          新建智能体
        </UButton>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
      <UCard v-for="agent in agents" :key="agent.uuid" class="border border-[var(--border-color)]">
        <div class="space-y-3">
          <div class="flex items-start justify-between gap-3">
            <div>
              <div class="text-base font-medium text-[var(--text-primary)]">{{ agent.name }}</div>
              <div class="text-xs text-[var(--text-secondary)]">{{ agent.key }}</div>
            </div>
            <UBadge :color="agent.status === 'active' ? 'success' : 'neutral'" variant="soft">
              {{ agent.status || 'unknown' }}
            </UBadge>
          </div>

          <p class="line-clamp-2 text-sm text-[var(--text-secondary)]">
            {{ agent.description || '暂无描述' }}
          </p>

          <div class="flex items-center gap-2">
            <UButton size="xs" variant="soft" icon="i-heroicons-chat-bubble-left-right" :to="localePath('/agent/sessions')">
              智能会话
            </UButton>
            <UButton size="xs" variant="soft" icon="i-heroicons-user-group" :to="localePath('/settings/ai/agent-teams')">
              创建团队
            </UButton>
            <UButton size="xs" variant="ghost" icon="i-heroicons-pencil-square" @click="editAgentName(agent.uuid, agent.name)">
              重命名
            </UButton>
          </div>
        </div>
      </UCard>
    </div>

    <UAlert
      v-if="errorText"
      color="error"
      variant="soft"
      icon="i-heroicons-exclamation-triangle"
      :title="errorText"
    />
  </div>
</template>

<script setup lang="ts">
import { useAgentManager } from '~/composables/agent/useAgentManager'

const localePath = useLocalePath()
const toast = useToast()
const { agents, fetchAgents, createAgent, updateAgent } = useAgentManager()

const errorText = ref('')

const load = async () => {
  try {
    errorText.value = ''
    await fetchAgents()
  } catch (e: any) {
    errorText.value = e?.message || '加载智能体失败'
  }
}

const createAgentQuick = async () => {
  const key = `agent_${Date.now().toString().slice(-6)}`
  try {
    await createAgent({
      key,
      name: `新智能体-${key.slice(-4)}`,
      description: '请在智能体管理中完善配置',
      status: 'active',
      meta: {},
    })
    toast.add({ title: '创建成功', description: '已创建默认智能体', color: 'success' })
    await load()
  } catch (e: any) {
    toast.add({ title: '创建失败', description: e?.message || '未知错误', color: 'error' })
  }
}

const editAgentName = async (uuid: string, current: string) => {
  const next = window.prompt('请输入新的智能体名称', current || '')
  if (!next || !next.trim()) return
  try {
    await updateAgent(uuid, { name: next.trim() })
    toast.add({ title: '更新成功', color: 'success' })
    await load()
  } catch (e: any) {
    toast.add({ title: '更新失败', description: e?.message || '未知错误', color: 'error' })
  }
}

onMounted(load)
</script>
